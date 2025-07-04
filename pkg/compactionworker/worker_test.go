package compactionworker

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	thanosstore "github.com/thanos-io/objstore"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockobjstore"
)

type MetastoreClientMock struct {
	*mockmetastorev1.MockCompactionServiceClient
	*mockmetastorev1.MockIndexServiceClient
}

func createTestWorker(t *testing.T, client MetastoreClient, compactFn compactFunc, bucket objstore.Bucket) *Worker {
	config := Config{
		JobConcurrency:     2,
		JobPollInterval:    100 * time.Millisecond,
		RequestTimeout:     time.Second,
		CleanupMaxDuration: time.Second,
		TempDir:            t.TempDir(),
	}

	worker, err := New(
		log.NewNopLogger(),
		config,
		client,
		bucket,
		prometheus.NewRegistry(),
		nil, // ruler
		nil, // exporter
	)

	require.NoError(t, err)
	worker.compactFn = compactFn
	return worker
}

func runWorker(w *Worker) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		svc := w.Service()
		_ = svc.StartAsync(ctx)
		_ = svc.AwaitRunning(ctx)
		time.Sleep(500 * time.Millisecond)
		svc.StopAsync()
		_ = svc.AwaitTerminated(ctx)
	}()

	wg.Wait()
}

func TestWorker_SuccessfulCompaction(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	block1ID := test.ULID("2024-01-01T10:00:00Z")
	block2ID := test.ULID("2024-01-01T11:00:00Z")
	compactedBlockID := test.ULID("2024-01-01T12:00:00Z")

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, storage objstore.Bucket, options ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		require.Len(t, blocks, 2)
		assert.Equal(t, block1ID, blocks[0].Id)
		assert.Equal(t, block2ID, blocks[1].Id)
		return []*metastorev1.BlockMeta{{Id: compactedBlockID, Tenant: 1, Shard: 1, CompactionLevel: 2}}, nil
	}

	w := createTestWorker(t, client, compactFn, bucket)

	job := &metastorev1.CompactionJob{
		Name:            "test-job",
		Tenant:          "test-tenant",
		Shard:           1,
		CompactionLevel: 1,
		SourceBlocks:    []string{block1ID, block2ID},
	}
	assignment := &metastorev1.CompactionJobAssignment{
		Name:  "test-job",
		Token: 12345,
	}

	metadata := []*metastorev1.BlockMeta{
		{Id: block1ID, Tenant: 1, Shard: 1},
		{Id: block2ID, Tenant: 1, Shard: 1},
	}
	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return req.JobCapacity > 0
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job},
		Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: metadata,
	}, nil).Once()

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return len(req.StatusUpdates) > 0 && req.StatusUpdates[0].Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Once()

	// Additional polls should return empty responses.
	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	runWorker(w)
}

func TestWorker_CompactionFailure(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	block1ID := test.ULID("2024-01-01T10:00:00Z")

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, storage objstore.Bucket, options ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		return nil, errors.New("compaction failed")
	}

	w := createTestWorker(t, client, compactFn, bucket)

	job := &metastorev1.CompactionJob{
		Name:            "test-job",
		Tenant:          "test-tenant",
		Shard:           1,
		CompactionLevel: 1,
		SourceBlocks:    []string{block1ID},
	}
	assignment := &metastorev1.CompactionJobAssignment{
		Name:  "test-job",
		Token: 12345,
	}

	metadata := []*metastorev1.BlockMeta{
		{Id: block1ID, Tenant: 1, Shard: 1},
	}
	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return req.JobCapacity > 0
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job},
		Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: metadata,
	}, nil).Once()

	bucket.EXPECT().IsObjNotFoundErr(mock.Anything).Return(false).Maybe()
	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	runWorker(w)
}

func TestWorker_JobCancellation(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	block1ID := test.ULID("2024-01-01T10:00:00Z")

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, storage objstore.Bucket, options ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		return nil, context.Canceled
	}

	w := createTestWorker(t, client, compactFn, bucket)

	job := &metastorev1.CompactionJob{
		Name:            "test-job",
		Tenant:          "test-tenant",
		Shard:           1,
		CompactionLevel: 1,
		SourceBlocks:    []string{block1ID},
	}
	assignment := &metastorev1.CompactionJobAssignment{
		Name:  "test-job",
		Token: 12345,
	}

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return req.JobCapacity > 0
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job},
		Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{{Id: block1ID, Tenant: 1, Shard: 1}},
	}, nil).Maybe()

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	runWorker(w)
}

func TestWorker_TombstoneHandling(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	sourceBlockID := test.ULID("2024-01-01T11:00:00Z")
	compactedBlockID := test.ULID("2024-01-01T12:00:00Z")
	oldBlock1ID := test.ULID("2024-01-01T08:00:00Z")
	oldBlock2ID := test.ULID("2024-01-01T09:00:00Z")

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, storage objstore.Bucket, options ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		return []*metastorev1.BlockMeta{{Id: compactedBlockID, Tenant: 1, Shard: 1, CompactionLevel: 2}}, nil
	}

	w := createTestWorker(t, client, compactFn, bucket)

	tombstones := []*metastorev1.Tombstones{{
		Blocks: &metastorev1.BlockTombstones{
			Name:            "test-tombstone",
			Tenant:          "test-tenant",
			Shard:           1,
			CompactionLevel: 1,
			Blocks:          []string{oldBlock1ID, oldBlock2ID},
		},
	}}

	job := &metastorev1.CompactionJob{
		Name:            "test-job",
		Tenant:          "test-tenant",
		Shard:           1,
		CompactionLevel: 1,
		SourceBlocks:    []string{sourceBlockID},
		Tombstones:      tombstones,
	}
	assignment := &metastorev1.CompactionJobAssignment{
		Name:  "test-job",
		Token: 12345,
	}

	metadata := []*metastorev1.BlockMeta{
		{Id: sourceBlockID, Tenant: 1, Shard: 1},
	}

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return req.JobCapacity > 0
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job},
		Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: metadata,
	}, nil).Once()

	bucket.EXPECT().Delete(mock.Anything, mock.MatchedBy(func(path string) bool {
		return (strings.Contains(path, oldBlock1ID) || strings.Contains(path, oldBlock2ID)) &&
			strings.Contains(path, "test-tenant")
	})).Return(nil).Times(2)

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return len(req.StatusUpdates) > 0 && req.StatusUpdates[0].Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Once()

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	runWorker(w)
}

func TestWorker_MetadataNotFound(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	missingBlockID := test.ULID("2024-01-01T10:00:00Z")

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, storage objstore.Bucket, options ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		t.Error("compactFn should not be called when metadata is not found")
		return nil, errors.New("should not be called")
	}

	w := createTestWorker(t, client, compactFn, bucket)

	job := &metastorev1.CompactionJob{
		Name:            "test-job",
		Tenant:          "test-tenant",
		Shard:           1,
		CompactionLevel: 1,
		SourceBlocks:    []string{missingBlockID},
	}
	assignment := &metastorev1.CompactionJobAssignment{
		Name:  "test-job",
		Token: 12345,
	}

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return req.JobCapacity > 0
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job},
		Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).Return((*metastorev1.GetBlockMetadataResponse)(nil), errors.New("metadata not found")).Once()

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	runWorker(w)
}

func TestWorker_ShardTombstoneHandling(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	sourceBlockID := test.ULID("2024-01-01T11:00:00Z")
	compactedBlockID := test.ULID("2024-01-01T12:00:00Z")
	oldBlock1ID := test.ULID("2024-01-01T08:00:00Z")
	oldBlock2ID := test.ULID("2024-01-01T09:00:00Z")
	newBlock1ID := test.ULID("2024-01-01T10:30:00Z")
	newBlock2ID := test.ULID("2024-01-01T11:30:00Z")

	compactFn := func(ctx context.Context, blocks []*metastorev1.BlockMeta, storage objstore.Bucket, options ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
		return []*metastorev1.BlockMeta{
			{Id: compactedBlockID, Tenant: 1, Shard: 1, CompactionLevel: 2},
		}, nil
	}

	w := createTestWorker(t, client, compactFn, bucket)

	tombstoneTime := test.Time("2024-01-01T09:00:00Z")
	duration := time.Hour
	shardTombstone := &metastorev1.Tombstones{
		Shard: &metastorev1.ShardTombstone{
			Name:      "test-shard-tombstone",
			Tenant:    "test-tenant",
			Shard:     1,
			Timestamp: tombstoneTime.UnixNano(),
			Duration:  int64(duration),
		},
	}

	job := &metastorev1.CompactionJob{
		Name:            "test-job",
		Tenant:          "test-tenant",
		Shard:           1,
		CompactionLevel: 1,
		SourceBlocks:    []string{sourceBlockID},
		Tombstones:      []*metastorev1.Tombstones{shardTombstone},
	}
	assignment := &metastorev1.CompactionJobAssignment{
		Name:  "test-job",
		Token: 12345,
	}

	metadata := []*metastorev1.BlockMeta{
		{Id: sourceBlockID, Tenant: 1, Shard: 1},
	}

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return req.JobCapacity > 0
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{
		CompactionJobs: []*metastorev1.CompactionJob{job},
		Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
	}, nil).Once()

	indexClient.EXPECT().GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.GetBlockMetadataResponse{
		Blocks: metadata,
	}, nil).Once()

	expectedDir := block.BuildObjectDir("test-tenant", 1)
	bucket.EXPECT().Iter(mock.Anything, expectedDir, mock.Anything, mock.Anything).Run(
		func(ctx context.Context, dir string, fn func(string) error, options ...thanosstore.IterOption) {
			blockPaths := []string{
				block.BuildObjectPath("test-tenant", 1, 1, oldBlock1ID),   // Should be deleted
				block.BuildObjectPath("test-tenant", 1, 1, oldBlock2ID),   // Should be deleted
				block.BuildObjectPath("test-tenant", 1, 1, newBlock1ID),   // SkipAll
				block.BuildObjectPath("test-tenant", 1, 1, newBlock2ID),   //
				block.BuildObjectPath("test-tenant", 1, 1, sourceBlockID), //
			}
			for _, path := range blockPaths {
				if err := fn(path); err != nil {
					return // Return(filepath.SkipAll).Once()
				}
			}
		}).Return(filepath.SkipAll).Once()

	bucket.EXPECT().Delete(mock.Anything, block.BuildObjectPath("test-tenant", 1, 1, oldBlock1ID)).Return(nil).Once()
	bucket.EXPECT().Delete(mock.Anything, block.BuildObjectPath("test-tenant", 1, 1, oldBlock2ID)).Return(nil).Once()
	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.MatchedBy(func(req *metastorev1.PollCompactionJobsRequest) bool {
		return len(req.StatusUpdates) > 0 && req.StatusUpdates[0].Status == metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS
	}), mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Once()

	compactionClient.EXPECT().PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	runWorker(w)
}

var skipCompactionFn = func(context.Context, []*metastorev1.BlockMeta, objstore.Bucket, ...block.CompactionOption) ([]*metastorev1.BlockMeta, error) {
	return nil, nil
}

func TestWorker_CleanupMaxDurationAtShutdown(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	compactionClient := mockmetastorev1.NewMockCompactionServiceClient(t)
	indexClient := mockmetastorev1.NewMockIndexServiceClient(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: compactionClient,
		MockIndexServiceClient:      indexClient,
	}

	config := Config{
		JobConcurrency:     1,
		JobPollInterval:    100 * time.Millisecond,
		RequestTimeout:     time.Second,
		CleanupMaxDuration: 15 * time.Second,
		TempDir:            t.TempDir(),
	}

	worker, err := New(
		log.NewNopLogger(),
		config,
		client,
		bucket,
		nil, // registry
		nil, // ruler
		nil, // exporter
	)
	require.NoError(t, err)
	worker.compactFn = skipCompactionFn

	job := &metastorev1.CompactionJob{Name: "test-job"}
	assignment := &metastorev1.CompactionJobAssignment{Name: job.Name, Token: 12345}
	job.Tombstones = []*metastorev1.Tombstones{{
		Blocks: &metastorev1.BlockTombstones{
			Name:            "test-tombstone",
			Tenant:          "test-tenant",
			Shard:           1,
			CompactionLevel: 1,
			Blocks:          []string{"a", "b"},
		},
	}}

	var once sync.Once
	done := make(chan struct{})
	triggerShutdown := func(context.Context, *metastorev1.PollCompactionJobsRequest, ...grpc.CallOption) {
		once.Do(func() { close(done) })
	}

	compactionClient.EXPECT().
		PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).
		Run(triggerShutdown).
		Return(&metastorev1.PollCompactionJobsResponse{
			CompactionJobs: []*metastorev1.CompactionJob{job},
			Assignments:    []*metastorev1.CompactionJobAssignment{assignment},
		}, nil).Once()

	indexClient.EXPECT().
		GetBlockMetadata(mock.Anything, mock.Anything, mock.Anything).
		Return(&metastorev1.GetBlockMetadataResponse{}, nil).
		Once()

	var blocksDeleted atomic.Int32
	bucket.EXPECT().
		Delete(mock.Anything, mock.Anything).
		Run(func(context.Context, string) {
			blocksDeleted.Add(1)
			time.Sleep(100 * time.Millisecond)
		}).Return(nil).Times(2)

	compactionClient.EXPECT().
		PollCompactionJobs(mock.Anything, mock.Anything, mock.Anything).
		Return(&metastorev1.PollCompactionJobsResponse{}, nil).Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc := worker.Service()
	assert.NoError(t, svc.StartAsync(ctx))
	assert.NoError(t, svc.AwaitRunning(ctx))

	// Wait for the job to be polled and shutdown immediately.
	<-done
	svc.StopAsync()
	assert.NoError(t, svc.AwaitTerminated(ctx))

	require.Equal(t, 2, int(blocksDeleted.Load()))
}
