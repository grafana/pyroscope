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
	stalls[followerIdx].stall()

	// Write exactly one block through the leader while the follower is
	// stalled. It commits via the healthy majority (leader + the other,
	// non-stalled follower); timeoutLogStore on the stalled follower
	// abandons its attempt to replicate it after LogStoreTimeout, but the
	// goroutine performing the write is still running and will land it on
	// disk once released.
	//
	// Deliberately only one: hashicorp/raft's pipeline-mode replication
	// (which every follower graduates to once healthy, before we ever
	// stall anything) sends AppendEntries without waiting for individual
	// acks, handing each off to a decode goroutine over a small, bounded
	// channel. If more than one write is issued here, a second pipelined
	// send can block trying to hand off to that channel while the decode
	// goroutine is itself blocked reading the (stalled) response to the
	// first — and if the first response's failure then makes the decode
	// goroutine exit, nothing is ever left to unblock that handoff. That
	// is a real, pre-existing deadlock in raft's pipeline transport under
	// a slow follower with multiple in-flight requests, independent of
	// timeoutLogStore entirely; it's not what this test is about, so it
	// only ever needs to drive a single request into that state.
	blockID := ulid.MustNew(1, rand.Reader).String()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err := ms.Instances[leaderIdx].AddBlock(ctx, &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{Id: blockID},
	})
	cancel()
	require.NoError(t, err, "write should commit via the healthy majority")
	t.Logf("wrote block while follower %d was stalled", followerIdx)

	leaderInfo := nodeInfo(t, ms, leaderIdx)
	require.NotNil(t, leaderInfo)
	leaderCommit := leaderInfo.GetCommitIndex()
	t.Logf("leader commit index: %d", leaderCommit)

	// Writing through the leader only needs a majority (leader + the
	// other, non-stalled follower), so the write above can complete well
	// within LogStoreTimeout without the stalled follower's attempt ever
	// actually timing out — in which case it would just complete slowly
	// but successfully once released, and there would be nothing
	// abandoned to reconcile. Wait out multiple timeout windows here so
	// the follower's StoreLogs call has genuinely been abandoned —
	// reported as failed to raft, with its goroutine still running — and
	// raft has fallen back from pipeline mode to retrying it individually,
	// before releasing the stall.
	time.Sleep(4 * cfg.Raft.LogStoreTimeout)

	// Release the stall. The abandoned write's goroutine, still running
	// underneath, now completes and lands the entry on disk, out of band
	// from raft's view of it as failed. Without the reconciliation logic
	// in timeoutLogStore, raft's retry for that same index would hit
	// "non-monotonic log entries" and the follower would never catch up.
	stalls[followerIdx].release()

	// AppliedIndex (not just LastIndex) reaching the leader's commit index
	// is the definitive proof of recovery: it means the follower's log
	// store holds valid, correctly-ordered entries all the way through
	// (raft never got permanently stuck retrying a "non-monotonic log
	// entries" error), and its FSM successfully replayed every one of
	// them. Without the fix, this would time out: the retry for the first
	// abandoned index fails forever once the phantom write lands, and the
	// follower's applied index never advances past it.
	require.Eventually(t, func() bool {
		info := nodeInfo(t, ms, followerIdx)
		return info != nil && info.GetAppliedIndex() >= leaderCommit
	}, 30*time.Second, 200*time.Millisecond,
		"follower must catch up to the leader's commit index after the stalled writes land")

	// GetBlockMetadata is served by the IndexService, which always reads
	// through the leader by design (see Metastore.leaderRead); it cannot
	// be used to directly inspect a follower's state. Reading the block
	// back through the (still healthy, never-stalled) leader confirms the
	// write path as a whole remained consistent throughout.
	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	resp, err := ms.Instances[leaderIdx].GetBlockMetadata(readCtx, &metastorev1.GetBlockMetadataRequest{
		Blocks: &metastorev1.BlockList{Blocks: []string{blockID}},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetBlocks(), 1)

	// The cluster overall must still be healthy: further writes commit
	// normally through the leader, and the now-recovered (and now
	// pipelining again) follower keeps up with them.
	const moreBlocks = 10
	for i := 0; i < moreBlocks; i++ {
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = ms.Instances[leaderIdx].AddBlock(writeCtx, &metastorev1.AddBlockRequest{
			Block: &metastorev1.BlockMeta{Id: ulid.MustNew(uint64(i+2), rand.Reader).String()},
		})
		writeCancel()
		require.NoError(t, err)
	}

	leaderInfo = nodeInfo(t, ms, leaderIdx)
	require.NotNil(t, leaderInfo)
	require.Eventually(t, func() bool {
		info := nodeInfo(t, ms, followerIdx)
		return info != nil && info.GetAppliedIndex() >= leaderInfo.GetCommitIndex()
	}, 30*time.Second, 200*time.Millisecond,
		"recovered follower must keep up with further writes")
}

func nodeInfo(t *testing.T, ms MetastoreSet, idx int) *raftnodepb.NodeInfo {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := ms.Instances[idx].NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
	if err != nil {
		return nil
	}
	return resp.GetNode()
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
	store   raft.LogStore
	mu      sync.RWMutex
	stalled chan struct{}
}

func (s *stallInjector) stall() {
	s.mu.Lock()
	s.stalled = make(chan struct{})
	s.mu.Unlock()
}

func (s *stallInjector) release() {
	s.mu.Lock()
	if s.stalled != nil {
		close(s.stalled)
		s.stalled = nil
	}
	s.mu.Unlock()
}

func (s *stallInjector) awaitRelease() {
	s.mu.RLock()
	ch := s.stalled
	s.mu.RUnlock()
	if ch != nil {
		<-ch
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
