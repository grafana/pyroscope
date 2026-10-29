package compactionworker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
)

func TestResolveByteCapacity(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int64(500), resolveByteCapacity(500, 0.5, func() int64 { return 2000 }))
	assert.Equal(t, int64(1000), resolveByteCapacity(0, 0.5, func() int64 { return 2000 }))
	assert.Equal(t, int64(0), resolveByteCapacity(0, 0, func() int64 { return 2000 }))
	assert.Equal(t, int64(0), resolveByteCapacity(0, 0.5, func() int64 { return 0 }))
}

func TestEstimateJobBytes(t *testing.T) {
	t.Parallel()

	// Format0 dataset with TableOfContents: [profiles=0, tsdb=100, symbols=300], Size=1000
	// => TSDBBytes=200, SymbolsBytes=700 (total 900).
	blocks := []*metastorev1.BlockMeta{
		{
			Id:          "b1",
			Size:        2000,
			StringTable: []string{"", "tenant-a", "svc-a", "svc-b"},
			Datasets: []*metastorev1.Dataset{
				{Tenant: 1, Name: 2, TableOfContents: []uint64{0, 100, 300}, Size: 1000},
				{Tenant: 1, Name: 3, TableOfContents: []uint64{1000, 1100, 1200}, Size: 500},
			},
		},
		{
			Id:          "b2",
			Size:        3000,
			StringTable: []string{"", "tenant-a", "svc-a"},
			Datasets: []*metastorev1.Dataset{
				{Tenant: 1, Name: 2, TableOfContents: []uint64{0, 200, 500}, Size: 1500},
			},
		},
	}

	// Without small-object in-memory loading: max dataset is svc-a = (200+700) + (300+1000) = 2200.
	assert.Equal(t, int64(2200), estimateJobBytes(blocks, 1000))

	// With small-object threshold >= 3000: both objects (2000 + 3000 = 5000) are loaded into memory + max dataset (2200) = 7200.
	assert.Equal(t, int64(7200), estimateJobBytes(blocks, 8<<20))
}

func TestByteBudget_PauseAndOversizedJob(t *testing.T) {
	t.Parallel()

	budget := newByteBudget(100)
	require.True(t, budget.hasHeadroom())

	// First job takes 60 bytes immediately.
	rel1, err := budget.acquire(context.Background(), 60, nil)
	require.NoError(t, err)
	require.True(t, budget.hasHeadroom())

	// Second job needs 60 bytes (60 + 60 > 100), so it pauses.
	var paused2 atomic.Bool
	acquired2 := make(chan func())
	go func() {
		rel, acquireErr := budget.acquire(context.Background(), 60, func() {
			paused2.Store(true)
		})
		require.NoError(t, acquireErr)
		acquired2 <- rel
	}()

	require.Eventually(t, paused2.Load, time.Second, 5*time.Millisecond)
	// While a waiter is queued, budget reports no headroom for polling.
	require.False(t, budget.hasHeadroom())

	// Third job is oversized (250 > 100 limit); it must wait behind job 2 and run once inUse == 0.
	var paused3 atomic.Bool
	acquired3 := make(chan func())
	go func() {
		rel, acquireErr := budget.acquire(context.Background(), 250, func() {
			paused3.Store(true)
		})
		require.NoError(t, acquireErr)
		acquired3 <- rel
	}()
	require.Eventually(t, paused3.Load, time.Second, 5*time.Millisecond)

	// Releasing job 1 allows job 2 to proceed, while job 3 remains paused.
	rel1()
	rel2 := <-acquired2
	select {
	case <-acquired3:
		t.Fatal("oversized job 3 should remain paused while job 2 is running")
	default:
	}

	// Releasing job 2 brings inUse to 0, unblocking oversized job 3.
	rel2()
	rel3 := <-acquired3
	require.False(t, budget.hasHeadroom())

	rel3()
	require.True(t, budget.hasHeadroom())
}

func TestWorker_PausesOversizedJobAndStopsPollingUntilHeadroom(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	block1ID := test.ULID("2024-01-01T10:00:00Z")
	block2ID := test.ULID("2024-01-01T11:00:00Z")
	out1ID := test.ULID("2024-01-01T12:00:00Z")
	out2ID := test.ULID("2024-01-01T13:00:00Z")

	job1 := &metastorev1.CompactionJob{
		Name:            "job-1",
		Tenant:          "tenant-a",
		Shard:           1,
		CompactionLevel: 2,
		SourceBlocks:    []string{block1ID},
	}
	job2 := &metastorev1.CompactionJob{
		Name:            "job-2",
		Tenant:          "tenant-a",
		Shard:           1,
		CompactionLevel: 2,
		SourceBlocks:    []string{block2ID},
	}
	assign1 := &metastorev1.CompactionJobAssignment{Name: job1.Name, Token: 1}
	assign2 := &metastorev1.CompactionJobAssignment{Name: job2.Name, Token: 2}

	var runningStarted sync.Once
	runningBlockStarted := make(chan struct{})
	releaseRunning := make(chan struct{})
	var activeCompactions atomic.Int32
	var maxConcurrentCompactions atomic.Int32

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, _ objstore.Bucket, _ ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		cur := activeCompactions.Add(1)
		for {
			prev := maxConcurrentCompactions.Load()
			if cur <= prev || maxConcurrentCompactions.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer activeCompactions.Add(-1)

		first := false
		runningStarted.Do(func() {
			first = true
			close(runningBlockStarted)
		})
		if first {
			select {
			case <-releaseRunning:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []*metastorev1.BlockMeta{{Id: out1ID, Tenant: 1, Shard: 1, CompactionLevel: 3}}, nil
		}
		return []*metastorev1.BlockMeta{{Id: out2ID, Tenant: 1, Shard: 1, CompactionLevel: 3}}, nil
	}

	// 3 threads (so thread capacity > 0 even with 2 jobs), but byte capacity = 1000 bytes.
	// Each job is 800 bytes, so job-2 must pause while job-1 runs, and the worker
	// must report JobCapacity == 0 on polls while job-2 is paused, despite having a free thread!
	cfg := Config{
		JobConcurrency:     3,
		JobByteCapacity:    1000,
		JobPollInterval:    50 * time.Millisecond,
		RequestTimeout:     time.Second,
		CleanupMaxDuration: time.Second,
		TempDir:            t.TempDir(),
	}
	w, err := New(log.NewNopLogger(), cfg, client, bucket, prometheus.NewRegistry(), nil, nil)
	require.NoError(t, err)
	w.compactFn = compactFn

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.MatchedBy(func(req *metastorev1.GetBlockMetadataRequest) bool {
		return len(req.Blocks.Blocks) == 1 && req.Blocks.Blocks[0] == block1ID
	}), mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{Id: block1ID, Tenant: 1, Shard: 1, Size: 800}},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.MatchedBy(func(req *metastorev1.GetBlockMetadataRequest) bool {
		return len(req.Blocks.Blocks) == 1 && req.Blocks.Blocks[0] == block2ID
	}), mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{Id: block2ID, Tenant: 1, Shard: 1, Size: 800}},
	}, nil).Once()

	// Initial poll hands both jobs to the worker.
	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return len(req.StatusUpdates) == 0 && req.JobCapacity == 3
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job1, job2},
		Assignments:    []*metastorev1.CompactionJobAssignment{assign1, assign2},
	}, nil).Once()

	var (
		unblockOnce          sync.Once
		observedZeroCapPoll  atomic.Bool
		completedUpdatesSeen atomic.Int32
		done                 = make(chan struct{})
		doneOnce             sync.Once
	)

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req *metastorev1.PollCompactionJobsRequest, _ ...grpc.CallOption) (*metastorev1.PollCompactionJobsResponse, error) {
			inProgress := 0
			for _, u := range req.StatusUpdates {
				switch u.Status {
				case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
					inProgress++
				case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
					if completedUpdatesSeen.Add(1) == 2 {
						doneOnce.Do(func() { close(done) })
					}
				}
			}
			// While both jobs are active (1 running + 1 paused), byte headroom is exhausted
			// so JobCapacity must be 0 even though the 3rd worker thread is idle, and both leases are renewed.
			if inProgress == 2 && req.JobCapacity == 0 {
				select {
				case <-runningBlockStarted:
					observedZeroCapPoll.Store(true)
					unblockOnce.Do(func() { close(releaseRunning) })
				default:
				}
			}
			return &metastorev1.PollCompactionJobsResponse{
				Assignments: []*metastorev1.CompactionJobAssignment{assign1, assign2},
			}, nil
		}).Maybe()

	runWorker(t, w, done)

	assert.True(t, observedZeroCapPoll.Load(), "worker should poll with JobCapacity=0 while a job is paused waiting for byte capacity")
	assert.Equal(t, int32(1), maxConcurrentCompactions.Load(), "second job must be paused until first job releases byte capacity")
}
