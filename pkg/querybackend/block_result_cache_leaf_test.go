package querybackend

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// Block primary storage until the cache hit cancels execution. An empty primary
// bucket can otherwise win the race with an incomplete response under CI load.
type cacheLeafPrimaryBucket struct {
	objstore.Bucket
}

func (b *cacheLeafPrimaryBucket) Get(ctx context.Context, _ string) (io.ReadCloser, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *cacheLeafPrimaryBucket) GetRange(ctx context.Context, _ string, _, _ int64) (io.ReadCloser, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *cacheLeafPrimaryBucket) Attributes(ctx context.Context, _ string) (thanobjstore.ObjectAttributes, error) {
	<-ctx.Done()
	return thanobjstore.ObjectAttributes{}, ctx.Err()
}

func TestResultCacheRunsOnlyAtLeaves(t *testing.T) {
	req, first := cacheTestRequest()
	first.StringTable = []string{"tenant-a", "service-a", "service-b"}
	first.Datasets[0].Name = 1
	second := first.CloneVT()
	// The same physical block can have distinct dataset selections in leaves.
	second.Datasets[0].Name = 2
	req.QueryPlan = &queryv1.QueryPlan{Root: &queryv1.QueryNode{
		Type: queryv1.QueryNode_MERGE,
		Children: []*queryv1.QueryNode{
			{Type: queryv1.QueryNode_READ, Blocks: []*metastorev1.BlockMeta{first}},
			{Type: queryv1.QueryNode_READ, Blocks: []*metastorev1.BlockMeta{second}},
		},
	}}
	original := req.CloneVT()
	plan := req.QueryPlan
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	seedBlockCache(t, bucket, req, first)
	seedBlockCache(t, bucket, req, second)
	primary := &cacheLeafPrimaryBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket())}
	reader := NewBlockReader(log.NewNopLogger(), primary, nil, validation.MockDefaultOverrides())
	leaf, err := New(Config{}, log.NewNopLogger(), nil, nil, reader, bucket, resultCacheOverrides{enabled: true})
	require.NoError(t, err)
	// Any cache lookup at the MERGE node would be observable here.
	parentBucket := &delayedCacheBucket{Bucket: bucket, release: make(chan struct{}), started: make(chan struct{}, 1)}
	parent, err := New(Config{}, log.NewNopLogger(), nil, leaf, nil, parentBucket, resultCacheOverrides{enabled: true})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp, err := parent.Invoke(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.Reports, 1)
	require.Equal(t, []string{"cluster"}, resp.Reports[0].LabelNames.LabelNames)
	require.Zero(t, resp.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched())
	require.Empty(t, parentBucket.started)
	require.Empty(t, parent.resultCacheWrites)
	require.Empty(t, leaf.resultCacheWrites)
	// MERGE clears the request plan before cloning child requests, as on main.
	// The plan itself and the remaining request fields must stay unchanged.
	require.Nil(t, req.QueryPlan)
	require.True(t, proto.Equal(original.QueryPlan, plan), "execution must not mutate the query plan")
	original.QueryPlan = nil
	require.True(t, proto.Equal(original, req))
}

func TestLeafCacheFiltersTenantDatasetsWithoutMutatingPlan(t *testing.T) {
	req, selected := cacheTestRequest()
	selected.StringTable = []string{"tenant-a", "tenant-b"}
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	seedBlockCache(t, bucket, req, selected)
	// This dataset must be removed before computing the cache identity.
	selected.Datasets = append(selected.Datasets, &metastorev1.Dataset{Tenant: 1, MinTime: 100, MaxTime: 200})
	original := req.CloneVT()
	primary := &cacheLeafPrimaryBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket())}
	reader := NewBlockReader(log.NewNopLogger(), primary, nil, validation.MockDefaultOverrides())
	backend, err := New(Config{}, log.NewNopLogger(), nil, nil, reader, bucket, resultCacheOverrides{enabled: true})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp, err := backend.Invoke(ctx, req)
	require.NoError(t, err)
	require.Len(t, resp.Reports, 1)
	require.Equal(t, []string{"cluster"}, resp.Reports[0].LabelNames.LabelNames)
	require.Empty(t, backend.resultCacheWrites)
	require.True(t, proto.Equal(original, req))
}
