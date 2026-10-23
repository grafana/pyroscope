package querybackend

import (
	"context"
	"io"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

type resultCacheWriterBucket struct {
	objstore.Bucket
	upload func(context.Context, string, io.Reader) error
	closed atomic.Bool
}

func (b *resultCacheWriterBucket) Upload(ctx context.Context, name string, r io.Reader, _ ...thanobjstore.ObjectUploadOption) error {
	return b.upload(ctx, name, r)
}

func (b *resultCacheWriterBucket) Close() error {
	b.closed.Store(true)
	return b.Bucket.Close()
}

func newResultCacheWriterTestBackend(t *testing.T, bucket objstore.Bucket) *QueryBackend {
	t.Helper()
	q, err := New(Config{}, log.NewNopLogger(), nil, nil, nil, bucket, nil)
	require.NoError(t, err)
	require.NoError(t, q.starting(t.Context()))
	t.Cleanup(func() {
		q.resultCacheStop()
		q.resultCacheWriteStop()
		q.resultCacheWorkers.Wait()
	})
	return q
}

func TestResultCacheShutdownDrainsQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bucket := &resultCacheWriterBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket())}
		bucket.upload = func(ctx context.Context, name string, r io.Reader) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return bucket.Bucket.Upload(ctx, name, r)
			}
		}
		q := newResultCacheWriterTestBackend(t, bucket)
		for i := range 4 {
			q.enqueueResultCacheWrite(resultCacheWriteJob{queryType: resultCacheLabelNames, key: strconv.Itoa(i), reports: cacheTestResponse().Reports})
		}
		synctest.Wait()
		require.Len(t, q.resultCacheWrites, 2, "two uploads should be in flight")

		start := time.Now()
		require.NoError(t, q.stopping(nil))
		require.Equal(t, 2*time.Second, time.Since(start))
		require.Empty(t, q.resultCacheWrites)
		require.True(t, bucket.closed.Load())
		require.Equal(t, float64(4), testutil.ToFloat64(q.resultCacheMetrics.writes.WithLabelValues(resultCacheLabelNames, "L2", "success")))
		require.Zero(t, testutil.ToFloat64(q.resultCacheMetrics.writes.WithLabelValues(resultCacheLabelNames, "L2", "dropped")))
	})
}

func TestResultCacheShutdownUsesOverallTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var uploads, active atomic.Int32
		bucket := &resultCacheWriterBucket{Bucket: objstore.NewBucket(thanobjstore.NewInMemBucket())}
		bucket.upload = func(ctx context.Context, _ string, _ io.Reader) error {
			uploads.Add(1)
			active.Add(1)
			defer active.Add(-1)
			<-ctx.Done()
			return ctx.Err()
		}
		q := newResultCacheWriterTestBackend(t, bucket)
		for i := range resultCacheQueueSize {
			q.enqueueResultCacheWrite(resultCacheWriteJob{queryType: resultCacheLabelNames, key: strconv.Itoa(i)})
		}
		synctest.Wait()
		require.Equal(t, int32(resultCacheWorkers), active.Load())
		// Existing uploads exhaust their individual timeout during shutdown,
		// but subsequent uploads must not get another full 30-second budget.
		time.Sleep(10 * time.Second)

		start := time.Now()
		require.NoError(t, q.stopping(nil))
		require.Equal(t, 30*time.Second, time.Since(start))
		require.Zero(t, active.Load())
		require.Equal(t, int32(2*resultCacheWorkers), uploads.Load())
		require.Empty(t, q.resultCacheWrites)
		require.True(t, bucket.closed.Load())
		require.Equal(t, float64(uploads.Load()), testutil.ToFloat64(q.resultCacheMetrics.writes.WithLabelValues(resultCacheLabelNames, "L2", "error")))
		require.Equal(t, float64(resultCacheQueueSize)-float64(uploads.Load()), testutil.ToFloat64(q.resultCacheMetrics.writes.WithLabelValues(resultCacheLabelNames, "L2", "dropped")))
	})
}
