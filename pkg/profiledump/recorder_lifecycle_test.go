package profiledump

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestRecorderServiceAdmission(t *testing.T) {
	for _, start := range []bool{false, true} {
		t.Run(map[bool]string{false: "never started", true: "started"}[start], func(t *testing.T) {
			policies := &policySet{policies: map[string]Policy{"a": recorderPolicy(t, "{}", 1, 10)}}
			var uploads atomic.Int64
			r, err := NewRecorder(DefaultRecorderConfig(), Dependencies{Policies: policies, Now: func() time.Time { return recorderNow }, Upload: func(context.Context, string, string, io.Reader) error { uploads.Add(1); return nil }})
			require.NoError(t, err)
			require.Equal(t, services.New, r.State())
			require.Equal(t, DropShutdown, r.Capture(context.Background(), "a", candidate(nil)).Reason)
			require.Empty(t, r.tenants)
			assertReleased(t, r)
			if start {
				require.NoError(t, services.StartAndAwaitRunning(context.Background(), r))
				require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
			}
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
			require.Equal(t, DropShutdown, r.Capture(context.Background(), "a", candidate(nil)).Reason)
			require.Equal(t, int64(map[bool]int{false: 0, true: 2}[start]), uploads.Load())
			require.Empty(t, r.tenants)
			assertReleased(t, r)
		})
	}
}

func TestRecorderContinuesAfterUploadFailure(t *testing.T) {
	var calls atomic.Int64
	r, _, _ := recorderFixture(t, recorderTestConfig(), func(context.Context, string, string, io.Reader) error {
		if calls.Add(1) == 1 {
			return errors.New("storage failure")
		}
		return nil
	}, nil)
	for range 2 {
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	}
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "error")))
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "success")))
	assertReleased(t, r)
}

func TestRecorderDrainDeadlineAndRepeatedStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := recorderTestConfig()
		cfg.UploadTimeout = time.Hour
		started := make(chan context.Context, 1)
		proceed := make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, _, _ string, body io.Reader) error {
			started <- ctx
			<-proceed
			return discardUpload(ctx, "", "", body)
		}, nil)
		t.Cleanup(unblock)
		require.True(t, r.Capture(context.Background(), "a", candidate([]byte("uploading"))).Enqueued)
		uploadCtx := await(t, started)
		require.True(t, r.Capture(context.Background(), "a", candidate([]byte("queued"))).Enqueued)
		before := retained(r)
		r.StopAsync()
		synctest.Wait()
		time.Sleep(shutdownDrain - time.Nanosecond)
		require.NoError(t, uploadCtx.Err())
		for range 8 {
			go r.StopAsync()
		}
		synctest.Wait()
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		require.ErrorIs(t, uploadCtx.Err(), context.Canceled)
		require.Equal(t, before, retained(r))
		require.NotEqual(t, services.Terminated, r.State())
		require.Equal(t, DropShutdown, r.Capture(context.Background(), "a", candidate(nil)).Reason)
		unblock()
		require.NoError(t, r.AwaitTerminated(context.Background()))
		require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "canceled")))
		require.Zero(t, testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
		require.Equal(t, 2., testutil.ToFloat64(r.metrics.dropped.WithLabelValues("connect", string(DropShutdown))))
		assertReleased(t, r)
	})
}

// Span callbacks let tests pause preparation outside the admission lock.
type preparationObserver struct {
	start func()
	end   func()
}

func (p preparationObserver) OnStart(context.Context, sdktrace.ReadWriteSpan) {
	if p.start != nil {
		p.start()
	}
}
func (p preparationObserver) OnEnd(sdktrace.ReadOnlySpan) {
	if p.end != nil {
		p.end()
	}
}
func (preparationObserver) Shutdown(context.Context) error   { return nil }
func (preparationObserver) ForceFlush(context.Context) error { return nil }

type stopOnReservation struct {
	prometheus.Gauge
	stop func()
}

func (g stopOnReservation) Add(n float64) { g.Gauge.Add(n); g.stop() }

func TestRecorderStopJoinsPreparation(t *testing.T) {
	for _, phase := range []string{"before reservation", "after reservation", "after enqueue"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				entered, proceed := make(chan struct{}), make(chan struct{})
				unblock := sync.OnceFunc(func() { close(proceed) })
				pause := func() { close(entered); <-proceed }
				observer := preparationObserver{}
				if phase == "before reservation" {
					observer.start = pause
				} else {
					observer.end = pause
				}
				provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(observer))
				previous := otel.GetTracerProvider()
				otel.SetTracerProvider(provider)
				t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
				var uploads atomic.Int64
				r, _, _ := recorderFixture(t, recorderTestConfig(), func(context.Context, string, string, io.Reader) error { uploads.Add(1); return nil }, nil)
				t.Cleanup(unblock)
				if phase == "after reservation" {
					r.metrics.retained = stopOnReservation{Gauge: r.metrics.retained, stop: r.StopAsync}
				}
				result := make(chan Outcome, 1)
				go func() { result <- r.Capture(context.Background(), "a", candidate([]byte("owned"))) }()
				await(t, entered)
				r.StopAsync()
				synctest.Wait()
				require.NotEqual(t, services.Terminated, r.State(), "completion must join preparation")
				require.Equal(t, DropShutdown, r.Capture(context.Background(), "a", candidate(nil)).Reason)
				unblock()
				out := await(t, result)
				require.NoError(t, r.AwaitTerminated(context.Background()))
				if phase == "after enqueue" {
					require.True(t, out.Enqueued)
					require.Equal(t, int64(2), uploads.Load())
				} else {
					require.Equal(t, DropShutdown, out.Reason)
					require.Zero(t, uploads.Load())
				}
				assertReleased(t, r)
			})
		})
	}
}
