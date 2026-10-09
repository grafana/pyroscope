package querybackend

import (
	"context"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestBlockCacheExecutionPanicReturnsError(t *testing.T) {
	req, _ := cacheTestRequest()
	q := cacheTestBackend(objstore.NewBucket(thanobjstore.NewInMemBucket()), queryHandlerFunc(func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		panic("execution panic")
	}))
	_, err := q.executeBlocks(context.Background(), req)
	require.ErrorContains(t, err, "execution panic")
	require.Empty(t, q.resultCacheWrites)
}

func TestLeafCacheDoesNotCacheMissingBlock(t *testing.T) {
	req, block := cacheTestRequest()
	block.StringTable = []string{"tenant-a"}
	storage := objstore.NewBucket(thanobjstore.NewInMemBucket())
	cache := objstore.NewBucket(thanobjstore.NewInMemBucket())
	reader := NewBlockReader(log.NewNopLogger(), storage, nil, validation.MockDefaultOverrides())
	backend, err := New(Config{}, log.NewNopLogger(), nil, nil, reader, cache, resultCacheOverrides{enabled: true})
	require.NoError(t, err)
	resp, err := backend.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Empty(t, resp.Reports)
	require.Empty(t, backend.resultCacheWrites)
}

func TestBlockCacheGenerationPreservesFullWidth(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large generations require a 64-bit uint")
	}
	req, block := cacheTestRequest()
	identity, err := blockResultCacheIdentity(req, block)
	require.NoError(t, err)
	large := uint64(1)<<32 + 1
	overrides, err := validation.NewOverrides(validation.Limits{ResultCacheGeneration: uint(large)}, nil)
	require.NoError(t, err)
	generation := overrides.ResultCacheGeneration("tenant-a")
	require.Equal(t, uint(large), generation)
	first, err := blockResultCacheKey("tenant-a", 1, identity)
	require.NoError(t, err)
	second, err := blockResultCacheKey("tenant-a", generation, identity)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}
