package test

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/grafana/dskit/flagext"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/metastore"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

// TestMetastoreDiskStall_FollowerRecoversAfterStalledWriteLands reproduces
// the follower livelock fixed by timeoutLogStore's write reconciliation
// (see pkg/metastore/raftnode/logstore.go): a slow disk write on a
// follower is abandoned by the leader's replication retry logic once
// timeoutLogStore's deadline elapses, but the goroutine performing the
// write is not cancelled — it keeps running and eventually lands the
// entry on disk anyway.
//
// This is a different fault model from
// TestMetastoreDiskFailure_ClusterRecovery (#4892), whose faultInjector
// blocks and then *fails* the call outright, so the delayed write never
// lands. That test covers the leader-stuck-forever scenario #4892 fixed,
// but its fault model can never exercise the bug fixed here: the real
// raft-wal failure mode is block-then-*succeed* — the write is real, just
// late. Once it lands, raft's retry for the same index hits an
// already-persisted entry, which a monotonic store like raft-wal rejects
// outright ("non-monotonic log entries"). Without reconciliation, that is
// a permanent livelock: the follower is stuck reporting "too far behind"
// forever, not just until the next heartbeat.
//
// Trailing-log and snapshot-threshold settings here are left at their
// (generous) defaults, and few enough entries are written that no
// InstallSnapshot ever triggers — recovery must come from ordinary log
// replication reconciling with what the stalled write already persisted,
// not from the leader happening to compact past the stuck index.
func TestMetastoreDiskStall_FollowerRecoversAfterStalledWriteLands(t *testing.T) {
	const clusterSize = 3

	stalls := make([]*stallInjector, clusterSize)
	for i := range stalls {
		stalls[i] = &stallInjector{}
	}

	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)
	cfg.Raft.LogStoreTimeout = 500 * time.Millisecond

	// Wire each node's log store through its stall injector.
	// NewMetastoreSet creates nodes 0..n-1 in order, each calling
	// LogStoreMiddleware during init.
	var mu sync.Mutex
	nodeIdx := 0
	cfg.Raft.LogStoreMiddleware = func(store raft.LogStore) raft.LogStore {
		mu.Lock()
		i := nodeIdx
		nodeIdx++
		mu.Unlock()
		stalls[i].store = store
		return stalls[i]
	}

	ms := NewMetastoreSet(t, cfg, clusterSize, memory.NewInMemBucket())
	defer func() {
		for _, s := range stalls {
			s.release()
		}
		ms.Close()
	}()

	leaderIdx := findLeader(t, ms)
	followerIdx := (leaderIdx + 1) % clusterSize
	t.Logf("leader is node %d, stalling follower node %d", leaderIdx, followerIdx)

	// Stall the follower's log store: its next StoreLogs call blocks, then
	// proceeds to the real underlying store once released.
	stall := stalls[followerIdx].stall()

	// Write one block through the leader while the follower is
	// stalled. It commits via the healthy majority (leader + the other,
	// non-stalled follower); timeoutLogStore on the stalled follower
	// abandons its attempt to replicate it after LogStoreTimeout, but the
	// goroutine performing the write is still running and will land it on
	// disk once released.
	blockID := ulid.MustNew(1, rand.Reader).String()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err := ms.Instances[leaderIdx].AddBlock(ctx, &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{Id: blockID},
	})
	cancel()
	require.NoError(t, err, "write should commit via the healthy majority")
	t.Logf("wrote block while follower %d was stalled", followerIdx)

	// The write must reach the stalled follower before the timeout window
	// starts. Otherwise releasing the stall later could let an ordinary,
	// non-abandoned write through and make this regression test pass without
	// exercising reconciliation.
	waitForStall(t, stall)
	time.Sleep(cfg.Raft.LogStoreTimeout + 100*time.Millisecond)

	// Release the stall. The abandoned write's goroutine, still running
	// underneath, now completes and lands the entry on disk, out of band
	// from raft's view of it as failed. Without the reconciliation logic
	// in timeoutLogStore, raft's retry for that same index would hit
	// "non-monotonic log entries" and the follower would never catch up.
	stalls[followerIdx].release()

	// A second append proves that recovery is ordinary log replication, not a
	// coincidental catch-up to the first committed entry.
	require.Equal(t, leaderIdx, findLeader(t, ms), "the original leader must remain in place")
	block2ID := ulid.MustNew(2, rand.Reader).String()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	_, err = ms.Instances[leaderIdx].AddBlock(ctx, &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{Id: block2ID},
	})
	cancel()
	require.NoError(t, err, "write after releasing the stalled follower should commit")

	infoCtx, infoCancel := context.WithTimeout(context.Background(), time.Second)
	leaderInfo, err := ms.Instances[leaderIdx].NodeInfo(infoCtx, &raftnodepb.NodeInfoRequest{})
	infoCancel()
	require.NoError(t, err)
	targetIndex := leaderInfo.GetNode().GetCommitIndex()

	// AppliedIndex reaching the captured post-recovery target proves the
	// follower reconciled the late write and then replicated a new entry.
	require.Eventually(t, func() bool {
		followerCtx, followerCancel := context.WithTimeout(context.Background(), time.Second)
		followerInfo, followerErr := ms.Instances[followerIdx].NodeInfo(followerCtx, &raftnodepb.NodeInfoRequest{})
		followerCancel()
		return followerErr == nil && followerInfo.GetNode().GetAppliedIndex() >= targetIndex
	}, 30*time.Second, 100*time.Millisecond,
		"follower must apply the post-recovery commit index")

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	response, err := ms.Instances[leaderIdx].GetBlockMetadata(readCtx, &metastorev1.GetBlockMetadataRequest{
		Blocks: &metastorev1.BlockList{Blocks: []string{blockID, block2ID}},
	})
	readCancel()
	require.NoError(t, err)
	require.Len(t, response.Blocks, 2)
}

// stallInjector wraps a raft.LogStore and can be switched to a "stalled"
// mode where StoreLog/StoreLogs block until released, then proceed to the
// real underlying store — i.e. the write is genuinely delayed, not
// failed. This models a slow disk.
//
// This is deliberately different from faultInjector (used by
// TestMetastoreDiskFailure_ClusterRecovery), which blocks and then fails
// the call outright: that models a disk that never completes the write,
// while stallInjector models one that eventually does, which is the
// scenario timeoutLogStore's write reconciliation logic exists for.
type stallInjector struct {
	store raft.LogStore
	mu    sync.Mutex
	gate  *stall
}

type stall struct {
	release chan struct{}
	entered chan struct{}

	releaseOnce sync.Once
	enteredOnce sync.Once
}

func (s *stallInjector) stall() *stall {
	stalled := &stall{release: make(chan struct{}), entered: make(chan struct{})}
	s.mu.Lock()
	s.gate = stalled
	s.mu.Unlock()
	return stalled
}

func (s *stallInjector) release() {
	s.mu.Lock()
	stalled := s.gate
	s.gate = nil
	s.mu.Unlock()
	if stalled != nil {
		stalled.releaseOnce.Do(func() { close(stalled.release) })
	}
}

func (s *stallInjector) awaitRelease() {
	s.mu.Lock()
	stalled := s.gate
	s.mu.Unlock()
	if stalled != nil {
		stalled.enteredOnce.Do(func() { close(stalled.entered) })
		<-stalled.release
	}
}

func waitForStall(t *testing.T, stalled *stall) {
	t.Helper()
	select {
	case <-stalled.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for write to enter stalled log store")
	}
}

func (s *stallInjector) FirstIndex() (uint64, error)            { return s.store.FirstIndex() }
func (s *stallInjector) LastIndex() (uint64, error)             { return s.store.LastIndex() }
func (s *stallInjector) GetLog(idx uint64, log *raft.Log) error { return s.store.GetLog(idx, log) }
func (s *stallInjector) DeleteRange(min, max uint64) error      { return s.store.DeleteRange(min, max) }

func (s *stallInjector) IsMonotonic() bool {
	if m, ok := s.store.(raft.MonotonicLogStore); ok {
		return m.IsMonotonic()
	}
	return false
}

func (s *stallInjector) StoreLog(log *raft.Log) error {
	return s.StoreLogs([]*raft.Log{log})
}

func (s *stallInjector) StoreLogs(logs []*raft.Log) error {
	s.awaitRelease()
	return s.store.StoreLogs(logs)
}
