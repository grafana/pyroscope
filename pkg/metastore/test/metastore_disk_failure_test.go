package test

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/grafana/dskit/flagext"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
)

// TestMetastoreDiskFailure_ClusterRecovery verifies that a full Pyroscope
// metastore cluster recovers after the leader's disk becomes slow. It uses
// the LogStoreMiddleware hook to inject a blocking fault on the
// leader's log store, then verifies a new leader is elected and the cluster
// can serve both reads and writes.
func TestMetastoreDiskFailure_ClusterRecovery(t *testing.T) {
	const clusterSize = 3

	faults := make([]*faultInjector, clusterSize)
	for i := range faults {
		faults[i] = &faultInjector{}
	}

	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)

	// Wire each node's log store through its fault injector.
	// NewMetastoreSet creates nodes 0..n-1 in order, each calling
	// LogStoreMiddleware during init.
	var mu sync.Mutex
	nodeIdx := 0
	cfg.Raft.LogStoreMiddleware = func(store raft.LogStore) raft.LogStore {
		mu.Lock()
		i := nodeIdx
		nodeIdx++
		mu.Unlock()
		faults[i].store = store
		return faults[i]
	}

	ms := NewMetastoreSet(t, cfg, clusterSize, memory.NewInMemBucket())
	defer func() {
		for _, f := range faults {
			f.Unblock()
		}
		ms.Close()
	}()

	leaderIdx := findLeader(t, ms)
	t.Logf("leader is node %d", leaderIdx)

	// Write a block through the leader to confirm the cluster is healthy.
	block1ID := ulid.MustNew(1, rand.Reader).String()
	_, err := ms.Instances[leaderIdx].AddBlock(context.Background(), &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{Id: block1ID},
	})
	require.NoError(t, err)
	t.Logf("wrote block %s through leader", block1ID)

	// Inject blocking disk fault on the leader.
	faults[leaderIdx].SetBlocked()
	t.Log("injected blocking disk fault on leader")

	// Write to the faulted leader should fail (or hang until timeout).
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()
	_, err = ms.Instances[leaderIdx].AddBlock(writeCtx, &metastorev1.AddBlockRequest{
		Block: &metastorev1.BlockMeta{Id: ulid.MustNew(2, rand.Reader).String()},
	})
	require.Error(t, err)
	t.Logf("write to faulted leader failed: %v", err)

	// Wait for a new leader to be elected.
	var newLeaderIdx int
	require.Eventually(t, func() bool {
		for i := range ms.Instances {
			if i == leaderIdx {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			info, infoErr := ms.Instances[i].NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
			cancel()
			if infoErr != nil {
				continue
			}
			if info.GetNode().GetState() == "Leader" {
				newLeaderIdx = i
				return true
			}
		}
		return false
	}, 30*time.Second, 500*time.Millisecond,
		"a new leader should be elected after disk failure")
	t.Logf("new leader elected: node %d", newLeaderIdx)

	// Unblock the faulted node so it can rejoin as a healthy follower.
	// This stabilizes the cluster for subsequent operations.
	faults[leaderIdx].Unblock()
	t.Log("unblocked faulted node")

	// Write a new block through the new leader.
	block2ID := ulid.MustNew(3, rand.Reader).String()
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := ms.Instances[newLeaderIdx].AddBlock(ctx, &metastorev1.AddBlockRequest{
			Block: &metastorev1.BlockMeta{Id: block2ID},
		})
		return err == nil
	}, 15*time.Second, 200*time.Millisecond,
		"should be able to write through new leader")
	t.Logf("wrote block %s through new leader", block2ID)

	// Read back both blocks to verify data consistency.
	var resp *metastorev1.GetBlockMetadataResponse
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		resp, err = ms.Instances[newLeaderIdx].GetBlockMetadata(ctx, &metastorev1.GetBlockMetadataRequest{
			Blocks: &metastorev1.BlockList{
				Blocks: []string{block1ID, block2ID},
			},
		})
		return err == nil && len(resp.Blocks) == 2
	}, 15*time.Second, 200*time.Millisecond,
		"both blocks should be readable after recovery")
	t.Log("cluster recovered: both blocks readable")
}

func findLeader(t *testing.T, ms MetastoreSet) int {
	t.Helper()
	for i := range ms.Instances {
		info, err := ms.Instances[i].NodeInfo(context.Background(), &raftnodepb.NodeInfoRequest{})
		if err != nil {
			continue
		}
		if info.GetNode().GetState() == "Leader" {
			return i
		}
	}
	t.Fatal("no leader found")
	return -1
}

// faultInjector wraps a raft.LogStore and can be switched to blocking mode.
type faultInjector struct {
	store   raft.LogStore
	mu      sync.RWMutex
	blocked chan struct{}
}

func (f *faultInjector) SetBlocked() {
	f.mu.Lock()
	f.blocked = make(chan struct{})
	f.mu.Unlock()
}

func (f *faultInjector) Unblock() {
	f.mu.Lock()
	if f.blocked != nil {
		close(f.blocked)
		f.blocked = nil
	}
	f.mu.Unlock()
}

func (f *faultInjector) checkFault() error {
	f.mu.RLock()
	ch := f.blocked
	f.mu.RUnlock()
	if ch != nil {
		<-ch
		return fmt.Errorf("simulated disk I/O error")
	}
	return nil
}

func (f *faultInjector) FirstIndex() (uint64, error)            { return f.store.FirstIndex() }
func (f *faultInjector) LastIndex() (uint64, error)             { return f.store.LastIndex() }
func (f *faultInjector) GetLog(idx uint64, log *raft.Log) error { return f.store.GetLog(idx, log) }
func (f *faultInjector) DeleteRange(min, max uint64) error      { return f.store.DeleteRange(min, max) }

func (f *faultInjector) StoreLog(log *raft.Log) error {
	if err := f.checkFault(); err != nil {
		return err
	}
	return f.store.StoreLog(log)
}

func (f *faultInjector) StoreLogs(logs []*raft.Log) error {
	if err := f.checkFault(); err != nil {
		return err
	}
	return f.store.StoreLogs(logs)
}
