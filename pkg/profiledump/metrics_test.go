package profiledump

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Assert wire label values independently of production instrumentation.
//
//nolint:goconst
func TestUploadOutcomeMetricsWithoutSampling(t *testing.T) {
	for _, result := range []string{"success", "error", "timeout", "canceled"} {
		t.Run(result, func(t *testing.T) {
			spans := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()), sdktrace.WithSpanProcessor(spans))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			started, release := make(chan struct{}), make(chan struct{})
			unblock := func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}
			r, _, clock := recorderFixture(t, recorderTestConfig(), func(_ context.Context, _, _ string, _ io.Reader) error {
				close(started)
				<-release
				switch result {
				case "error":
					return errors.New("storage failure")
				case "timeout":
					return fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
				case "canceled":
					return context.Canceled
				default:
					return nil
				}
			}, nil)
			t.Cleanup(unblock)
			c := candidate([]byte("native"))
			c.StartSpan = CaptureSpan(provider.Tracer("test"))
			out := r.Capture(context.Background(), "a", c)
			require.True(t, out.Enqueued)
			await(t, started)
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.candidates.WithLabelValues("connect", "enqueued")))
			require.Equal(t, float64(out.Size), testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "enqueued")))
			require.Zero(t, testutil.ToFloat64(r.metrics.queueItems))
			require.Zero(t, testutil.ToFloat64(r.metrics.queueBytes))
			require.Positive(t, testutil.ToFloat64(r.metrics.retained))
			clock.advance(2 * time.Second)
			unblock()
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
			require.Empty(t, spans.Started())
			require.Empty(t, spans.Ended())
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", result)))
			wantErrors, wantBytes := 1., 0.
			if result == "success" {
				wantErrors, wantBytes = 0, float64(out.Size)
			}
			require.Equal(t, wantErrors, testutil.ToFloat64(r.metrics.uploadErrors.WithLabelValues("connect")))
			require.Equal(t, wantBytes, testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
			require.Zero(t, testutil.ToFloat64(r.metrics.candidates.WithLabelValues("connect", "dropped")))
			var metric dto.Metric
			histogram, err := r.metrics.uploadDuration.GetMetricWithLabelValues("connect")
			require.NoError(t, err)
			require.NoError(t, histogram.(interface{ Write(*dto.Metric) error }).Write(&metric))
			require.EqualValues(t, 1, metric.GetHistogram().GetSampleCount())
			require.Equal(t, 2., metric.GetHistogram().GetSampleSum())
		})
	}
}
