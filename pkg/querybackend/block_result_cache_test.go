package querybackend

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
)

func TestBlockCacheMergesHitsAndUncachedBlocks(t *testing.T) {
	req, cached := cacheTestRequest()
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	seedBlockCache(t, bucket, req, cached)
	uncached := cached.CloneVT()
	uncached.Id = "block-b"
	uncached.CompactionLevel = 1
	req.QueryPlan = queryplan.Build([]*metastorev1.BlockMeta{cached, uncached}, 4, 20)
	q := cacheTestBackend(bucket, queryHandlerFunc(func(_ context.Context, execution *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		require.Same(t, req.QueryPlan, execution.QueryPlan)
		resp := cacheTestResponse()
		resp.Reports[0].LabelNames.LabelNames = []string{"service_name", "cluster"}
		return resp, nil
	}))
	resp, err := q.executeBlocks(context.Background(), req)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"cluster", "service_name"}, resp.Reports[0].LabelNames.LabelNames)
	require.Empty(t, q.resultCacheWrites)
}

func TestBlockCacheCorruptEntryFallsBack(t *testing.T) {
	req, block := cacheTestRequest()
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	key := seedBlockCache(t, bucket, req, block)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader([]byte("invalid protobuf"))))
	q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		return cacheTestResponse(), nil
	}))
	resp, err := q.executeBlocks(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"cluster"}, resp.Reports[0].LabelNames.LabelNames)
	require.Len(t, q.resultCacheWrites, 1)
}

func TestBlockCacheCallerCancellation(t *testing.T) {
	req, _ := cacheTestRequest()
	bucket := &delayedCacheBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket()), release: make(chan struct{})}
	q := cacheTestBackend(bucket, queryHandlerFunc(func(ctx context.Context, _ *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := q.executeBlocks(ctx, req)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, q.resultCacheWrites)
}

func TestBlockCacheIneligibleRequestsExecuteWithoutLookup(t *testing.T) {
	for _, name := range []string{"disabled", "diagnostics", "multi-tenant", "unsupported query", "no bucket"} {
		t.Run(name, func(t *testing.T) {
			req, _ := cacheTestRequest()
			originalPlan := req.QueryPlan.CloneVT()
			bucket := &delayedCacheBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket()), release: make(chan struct{}), started: make(chan struct{}, 1)}
			calls := 0
			q := cacheTestBackend(bucket, queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				calls++
				return &queryv1.InvokeResponse{}, nil
			}))
			switch name {
			case "disabled":
				q.resultCacheOverrides = resultCacheOverrides{}
			case "diagnostics":
				req.Options = &queryv1.InvokeOptions{CollectDiagnostics: true}
			case "multi-tenant":
				req.Tenant = append(req.Tenant, "tenant-b")
			case "unsupported query":
				req.Query = nil
			case "no bucket":
				q.resultCacheBucket = nil
			}
			_, err := q.executeBlocks(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Empty(t, bucket.started)
			require.Empty(t, q.resultCacheWrites)
			require.Equal(t, originalPlan, req.QueryPlan)
		})
	}
}

func TestBlockCacheFullQueueDoesNotDelayResponse(t *testing.T) {
	req, _ := cacheTestRequest()
	q := cacheTestBackend(objstore.NewBucket(thanobjstore.NewInMemBucket()), queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		return cacheTestResponse(), nil
	}))
	for range cap(q.resultCacheWrites) {
		q.resultCacheWrites <- resultCacheWriteJob{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := q.executeBlocks(ctx, req)
	require.NoError(t, err)
	require.Len(t, q.resultCacheWrites, cap(q.resultCacheWrites))
}
