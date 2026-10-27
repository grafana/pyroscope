package profiledump

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecorderQueueRejectionReleasesCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := recorderTestConfig()
		cfg.QueueCapacity = 1
		cfg.MaxCaptureBytes, cfg.MaxRetainedBytes = 4096, 4096
		proceed := make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, tenant, key string, body io.Reader) error {
			<-proceed
			return discardUpload(ctx, tenant, key, body)
		})
		t.Cleanup(unblock)
		payload := make([]byte, 512)
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), payload, r.now()))
		synctest.Wait()
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), payload, r.now()))
		// Three prepared captures fit the byte budget, but only one can wait in the queue.
		for range 2 {
			require.Equal(t, dropQueueFull, r.capture(context.Background(), "a", captureMetadata(), payload, r.now()))
		}
		unblock()
		synctest.Wait()
		require.Equal(t, 2., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("success")))
		// A leaked rejected reservation would leave too little capacity for this capture.
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), make([]byte, 3300), r.now()))
		synctest.Wait()
		require.Equal(t, 3., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("success")))
	})
}

func TestRecorderRetainsBuffersUntilUploadReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := recorderTestConfig()
		cfg.MaxCaptureBytes, cfg.MaxRetainedBytes = 1024, 1024
		started, proceed := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, _, _ string, body io.Reader) error {
			close(started)
			<-proceed // Models a provider which ignores cancellation.
			data, err := io.ReadAll(body)
			assert.Equal(t, bytes.Repeat([]byte("x"), 512), data)
			assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			return err
		})
		t.Cleanup(unblock)
		payload := bytes.Repeat([]byte("x"), 512)
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), payload, r.now()))
		await(t, started)
		time.Sleep(recorderUploadTimeout)
		synctest.Wait()
		_, reason := r.prepareCapture("a", captureMetadata(), payload, r.now())
		require.Equal(t, dropByteBudget, reason, "canceled upload owns its buffers until it returns")
		unblock()
		synctest.Wait()
	})
}

func TestRecorderFixedUploadConcurrency(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.Workers = 2
	cfg.QueueCapacity = 3
	started, proceed := make(chan struct{}, 5), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	var active atomic.Int64
	r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, tenant, key string, body io.Reader) error {
		n := active.Add(1)
		defer active.Add(-1)
		assert.LessOrEqual(t, n, int64(cfg.Workers))
		if strings.HasSuffix(key, ".pprof") {
			started <- struct{}{}
		}
		<-proceed
		return discardUpload(ctx, tenant, key, body)
	})
	t.Cleanup(unblock)
	for range cfg.Workers {
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), nil, r.now()))
		await(t, started)
	}
	for range cfg.QueueCapacity {
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), nil, r.now()))
	}
	require.Equal(t, dropQueueFull, r.capture(context.Background(), "a", captureMetadata(), nil, r.now()))
	require.Len(t, r.queue, cfg.QueueCapacity)
	require.Equal(t, int64(cfg.Workers), active.Load())
	unblock()
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	require.Zero(t, active.Load())
}
