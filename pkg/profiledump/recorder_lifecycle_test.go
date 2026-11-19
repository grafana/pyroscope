package profiledump

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
)

func TestRecorderServiceAdmission(t *testing.T) {
	policies := &policySet{policies: map[string]Policy{"a": recorderPolicy(t, "{}", 1)}}
	r, err := NewRecorder(DefaultRecorderConfig(), policies, discardUpload, "test", log.NewNopLogger(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r)) })
	r.now = func() time.Time { return recorderNow }
	series := &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{}}}
	r.CaptureSeries(context.Background(), "a", series)
	require.Zero(t, testutil.CollectAndCount(r.metrics.captures))
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), r))
	r.CaptureSeries(context.Background(), "a", series)
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	awaitUploads(t, r, 1)
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	r.CaptureSeries(context.Background(), "a", series)
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	require.Equal(t, 1, testutil.CollectAndCount(r.metrics.captures))
}

func TestRecorderCapacityRecoversAfterUpload(t *testing.T) {
	for _, failure := range []error{nil, errors.New("storage failure")} {
		name := "success"
		if failure != nil {
			name = "error"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := recorderTestConfig()
				cfg.MaxCaptureBytes, cfg.MaxRetainedBytes = 1024, 1024
				r, _, _ := recorderFixture(t, cfg, func(context.Context, string, string, io.Reader) error { return failure })
				for range 2 {
					// Each capture uses more than half the budget, exposing a leaked reservation.
					require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), make([]byte, 512), r.now()))
					synctest.Wait()
				}
				require.Equal(t, 2., testutil.ToFloat64(r.metrics.uploads.WithLabelValues(name)))
			})
		})
	}
}

func TestRecorderShutdownCancelsUploadsAndAbandonsQueue(t *testing.T) {
	started := make(chan context.Context, 1)
	var calls int
	var logs bytes.Buffer
	r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
		calls++
		started <- ctx
		<-ctx.Done()
		return ctx.Err()
	})
	r.logger = log.NewLogfmtLogger(&logs)
	require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), []byte("uploading"), r.now()))
	uploadCtx := await(t, started)
	require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), []byte("queued"), r.now()))
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	require.ErrorIs(t, uploadCtx.Err(), context.Canceled)
	require.Equal(t, 1, calls)
	require.Empty(t, logs.String(), "expected shutdown cancellation should not log upload errors")
	require.Zero(t, testutil.CollectAndCount(r.metrics.uploads))
}
