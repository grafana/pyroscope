package querybackend

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
)

type resultCacheOverrides struct {
	enabled    bool
	generation uint
}

func (o resultCacheOverrides) ResultCacheEnabled(string) bool    { return o.enabled }
func (o resultCacheOverrides) ResultCacheGeneration(string) uint { return o.generation }

type queryHandlerFunc func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)

func (f queryHandlerFunc) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return f(ctx, req)
}

type delayedCacheBucket struct {
	objstore.Bucket
	release <-chan struct{}
	started chan struct{}
}

func (b *delayedCacheBucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if b.started != nil {
		select {
		case b.started <- struct{}{}:
		default:
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.release:
		return b.Bucket.Get(ctx, key)
	}
}
func cacheTestRequest() (*queryv1.InvokeRequest, *metastorev1.BlockMeta) {
	block := &metastorev1.BlockMeta{Id: "block-a", CompactionLevel: 2, MinTime: 100, MaxTime: 200, Datasets: []*metastorev1.Dataset{{MinTime: 100, MaxTime: 200}}}
	return &queryv1.InvokeRequest{Tenant: []string{"tenant-a"}, StartTime: 0, EndTime: 300, LabelSelector: "{}", Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}}, QueryPlan: queryplan.Build([]*metastorev1.BlockMeta{block}, 4, 20)}, block
}
func cacheTestResponse() *queryv1.InvokeResponse {
	return &queryv1.InvokeResponse{Reports: []*queryv1.Report{{ReportType: queryv1.ReportType_REPORT_LABEL_NAMES, LabelNames: &queryv1.LabelNamesReport{Query: &queryv1.LabelNamesQuery{}, LabelNames: []string{"cluster"}}}}}
}

type testBlockCache struct {
	*resultCache
	reader QueryHandler
}

func cacheTestBackend(bucket objstore.Bucket, reader QueryHandler) *testBlockCache {
	return &testBlockCache{resultCache: &resultCache{resultCacheExecutionDelay: 15 * time.Millisecond, resultCacheBucket: bucket, resultCacheOverrides: resultCacheOverrides{enabled: true}, resultCacheMetrics: newResultCacheMetrics(nil), resultCacheWrites: make(chan resultCacheWriteJob, resultCacheQueueSize)}, reader: reader}
}

func (q *testBlockCache) executeBlocks(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	agg := newAggregator(req)
	for _, block := range req.QueryPlan.Root.Blocks {
		resp, err := q.executeBlock(ctx, req, block, func(ctx context.Context) (*queryv1.InvokeResponse, error) {
			return q.reader.Invoke(ctx, req)
		})
		if err != nil {
			return nil, err
		}
		if err := agg.aggregateResponse(resp); err != nil {
			return nil, err
		}
	}
	return agg.response(), nil
}
func seedBlockCache(t *testing.T, bucket objstore.Bucket, req *queryv1.InvokeRequest, block *metastorev1.BlockMeta) string {
	t.Helper()
	identity, err := blockResultCacheIdentity(req, block)
	require.NoError(t, err)
	key, err := blockResultCacheKey(req.Tenant[0], 0, identity)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Key: identity, Reports: cacheTestResponse().Reports})
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader(data)))
	return key
}
func TestBlockCacheHitAvoidsExecution(t *testing.T) {
	req, block := cacheTestRequest()
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	seedBlockCache(t, bucket, req, block)
	var calls atomic.Int32
	q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		calls.Add(1)
		return cacheTestResponse(), nil
	}))
	resp, err := q.executeBlocks(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"cluster"}, resp.Reports[0].LabelNames.LabelNames)
	// executeBlocks only returns after joining the (canceled) execution
	// goroutine, so no further calls can race with this assertion.
	require.Zero(t, calls.Load())
	require.Empty(t, q.resultCacheWrites)
}
func TestBlockCacheLateHitCancelsExecution(t *testing.T) {
	req, block := cacheTestRequest()
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	seedBlockCache(t, bucket, req, block)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	q := cacheTestBackend(&delayedCacheBucket{Bucket: bucket, release: release}, queryHandlerFunc(func(ctx context.Context, _ *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		started <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		return nil, ctx.Err()
	}))
	done := make(chan error, 1)
	go func() { _, err := q.executeBlocks(context.Background(), req); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("execution did not start")
	}
	close(release)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("hit did not return")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("execution was not canceled")
	}
	require.Empty(t, q.resultCacheWrites)
}
func TestBlockCacheSlowLookupDoesNotDelayExecution(t *testing.T) {
	req, _ := cacheTestRequest()
	release := make(chan struct{})
	bucket := &delayedCacheBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket()), release: release}
	q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		return cacheTestResponse(), nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := q.executeBlocks(ctx, req)
	require.NoError(t, err)
	require.Len(t, q.resultCacheWrites, 1)
}
func TestBlockCacheMissUploadsAsynchronously(t *testing.T) {
	req, block := cacheTestRequest()
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		return cacheTestResponse(), nil
	}))
	_, err := q.executeBlocks(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, q.resultCacheWrites, 1)
	identity, err := blockResultCacheIdentity(req, block)
	require.NoError(t, err)
	key, err := blockResultCacheKey(req.Tenant[0], 0, identity)
	require.NoError(t, err)
	exists, err := bucket.Exists(context.Background(), key)
	require.NoError(t, err)
	require.False(t, exists)
	ctx, cancel := context.WithCancel(context.Background())
	q.resultCacheWorkers.Add(1)
	go q.runResultCacheWriter(ctx, context.Background())
	t.Cleanup(func() { cancel(); q.resultCacheWorkers.Wait() })
	require.Eventually(t, func() bool { exists, _ := bucket.Exists(context.Background(), key); return exists }, time.Second, time.Millisecond)
}
func TestBlockCacheBypassesLowerLevelsAndPartialBlocks(t *testing.T) {
	for _, level := range []uint32{0, 1, 2, 3} {
		req, block := cacheTestRequest()
		block.CompactionLevel = level
		if level == 2 {
			req.StartTime = 150
		}
		req.QueryPlan = queryplan.Build([]*metastorev1.BlockMeta{block}, 4, 20)
		bucket := &delayedCacheBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket()), release: make(chan struct{}), started: make(chan struct{}, 1)}
		q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
			return cacheTestResponse(), nil
		}))
		_, err := q.executeBlocks(context.Background(), req)
		require.NoError(t, err)
		require.Empty(t, bucket.started)
		require.Empty(t, q.resultCacheWrites)
	}
}
func TestBlockCacheIdentity(t *testing.T) {
	req, block := cacheTestRequest()
	first, err := blockResultCacheIdentity(req, block)
	require.NoError(t, err)
	req.StartTime = -100
	req.EndTime = 400
	second, err := blockResultCacheIdentity(req, block)
	require.NoError(t, err)
	require.True(t, proto.Equal(first, second))
	key, err := blockResultCacheKey("a", 1, first)
	require.NoError(t, err)
	other, err := blockResultCacheKey("b", 1, first)
	require.NoError(t, err)
	require.NotEqual(t, key, other)
	other, err = blockResultCacheKey("a", 2, first)
	require.NoError(t, err)
	require.NotEqual(t, key, other)
	block.Id = "block-b"
	second, err = blockResultCacheIdentity(req, block)
	require.NoError(t, err)
	require.False(t, proto.Equal(first, second))
}
