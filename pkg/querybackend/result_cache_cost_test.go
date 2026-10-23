package querybackend

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

func TestBlockCachePreservesExecutionCost(t *testing.T) {
	req, block := cacheTestRequest()
	bucket := objstore.NewBucket(thanobjstore.NewInMemBucket())
	q := cacheTestBackend(bucket, nil)
	q.resultCacheExecutionDelay = 0
	resp := cacheTestResponse()
	resp.Diagnostics = &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{BytesFetched: 1234}}}
	_, err := q.executeBlock(context.Background(), req, block, func(context.Context) (*queryv1.InvokeResponse, error) { return resp, nil })
	require.NoError(t, err)
	job := <-q.resultCacheWrites
	require.Equal(t, uint64(1234), job.bytesFetched)
	q.uploadResultCacheEntry(context.Background(), job)
	q.resultCacheExecutionDelay = time.Hour
	hit, err := q.executeBlock(context.Background(), req, block, func(context.Context) (*queryv1.InvokeResponse, error) { panic("unexpected execution") })
	require.NoError(t, err)
	require.Equal(t, uint64(1234), hit.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched())
}

func TestBlockCacheHitDoesNotWaitForLosingExecution(t *testing.T) {
	req, block := cacheTestRequest()
	lookupRelease := make(chan struct{})
	bucket := &delayedCacheBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket()), release: lookupRelease}
	seedBlockCache(t, bucket.Bucket, req, block)
	q := cacheTestBackend(bucket, nil)
	q.resultCacheExecutionDelay = 0
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() { close(release); <-finished }()
	done := make(chan error, 1)
	go func() {
		_, err := q.executeBlock(context.Background(), req, block, func(context.Context) (*queryv1.InvokeResponse, error) {
			close(started)
			defer close(finished)
			<-release // Simulate a reader that does not promptly observe cancellation.
			return cacheTestResponse(), nil
		})
		done <- err
	}()
	<-started
	close(lookupRelease)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("cache hit waited for losing execution")
	}
	require.Empty(t, q.resultCacheWrites)
}
