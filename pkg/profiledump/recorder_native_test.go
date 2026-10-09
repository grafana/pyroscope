package profiledump

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestRecorderNativePayloads(t *testing.T) {
	compressed, err := os.ReadFile("../pprof/testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	plain, err := io.ReadAll(gz)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	for _, tc := range []struct {
		name, encoding string
		payload        []byte
	}{
		{"gzip", "gzip", compressed}, {"uncompressed", "identity", plain},
		{"empty", "identity", []byte{}}, {"malformed", "identity", []byte{0, 0xff, 1}},
		{"malformed gzip", "gzip", []byte{0x1f, 0x8b, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			var payload, metadata []byte
			r, policies, _ := recorderFixture(t, recorderTestConfig(), func(_ context.Context, tenant, key string, body io.Reader) error {
				require.Equal(t, "3648", tenant)
				keys = append(keys, key)
				raw, err := io.ReadAll(body)
				if strings.HasSuffix(key, ".pprof") {
					payload = raw
				} else {
					metadata = raw
				}
				return err
			}, nil)
			policies.set("3648", recorderPolicy(t, "{}", 1, 10))
			c := candidate(tc.payload)
			c.Metadata.PayloadEncoding = tc.encoding
			out := r.Capture(context.Background(), "3648", c)
			require.True(t, out.Enqueued)
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
			parsed, err := ParseNativeObjectKey(out.ObjectKey)
			require.NoError(t, err)
			require.Equal(t, []string{parsed.PayloadKey, parsed.MetadataKey}, keys)
			require.Contains(t, out.ObjectKey, "/native/3648/")
			require.Equal(t, tc.payload, payload)
			m, err := ReadNativeMetadata(bytes.NewReader(metadata), parsed.MetadataKey)
			require.NoError(t, err)
			require.Equal(t, tc.encoding, m.PayloadEncoding)
			require.Equal(t, int64(len(payload)), m.PayloadSize)
			require.Equal(t, int64(len(payload)+len(metadata)), out.Size)
			assertReleased(t, r)
		})
	}
}

func TestNativeCaptureSizeOverflow(t *testing.T) {
	size, err := nativeCaptureSize(math.MaxInt64-1, 1, math.MaxInt64)
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), size)
	for _, sizes := range [][3]int64{
		{math.MaxInt64, 1, math.MaxInt64}, {1, math.MaxInt64, math.MaxInt64},
		{-1, 1, 10}, {1, -1, 10}, {0, 1, -1}, {5, 6, 10},
	} {
		_, err := nativeCaptureSize(sizes[0], sizes[1], sizes[2])
		require.Error(t, err)
	}
	r, _, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	r.cfg.MaxRetainedBytes = math.MaxInt64
	size, ok := r.reservationSize(math.MaxInt64-itemReservation-1, 1)
	require.True(t, ok)
	require.Equal(t, int64(math.MaxInt64), size)
}

func TestRecorderNativePairUploadResults(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		stage        int
		failure      error
		delay        time.Duration
	}{
		{name: "complete", result: "success"},
		{name: "payload failure", result: "error", stage: 1, failure: errors.New("payload failed")},
		{name: "sidecar failure", result: "error", stage: 2, failure: errors.New("sidecar failed")},
		{name: "payload cancellation", result: "canceled", stage: 1, failure: context.Canceled},
		{name: "sidecar cancellation", result: "canceled", stage: 2, failure: context.Canceled},
		{name: "payload timeout after acceptance", result: "timeout", stage: 1, delay: 2 * time.Second},
		{name: "sidecar timeout after acceptance", result: "timeout", stage: 2, delay: time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := recorderTestConfig()
				cfg.UploadTimeout = 2 * time.Second
				var calls int
				var deadline time.Time
				var acceptedBytes int
				var r *Recorder
				r, _, _ = recorderFixture(t, cfg, func(ctx context.Context, _, key string, body io.Reader) error {
					calls++
					require.Positive(t, retained(r), "worker must retain ownership during both calls")
					d, ok := ctx.Deadline()
					require.True(t, ok)
					if calls == 1 {
						require.True(t, strings.HasSuffix(key, ".pprof"))
						deadline = d
					} else {
						require.Equal(t, 2, calls)
						require.True(t, strings.HasSuffix(key, ".json"))
						require.Equal(t, deadline, d, "both uploads must use the same capture deadline")
						require.Equal(t, time.Second, time.Until(d), "payload time consumes the pair budget")
					}
					raw, err := io.ReadAll(body)
					require.NoError(t, err)
					// A provider may accept an object before returning an error or late success.
					acceptedBytes += len(raw)
					if calls == tc.stage {
						time.Sleep(tc.delay)
						if tc.delay > 0 {
							<-ctx.Done()
						}
						return tc.failure
					}
					if calls == 1 {
						time.Sleep(time.Second)
					}
					return nil
				}, nil)
				out := r.Capture(context.Background(), "a", candidate([]byte("native")))
				require.True(t, out.Enqueued)
				require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
				wantCalls := 2
				if tc.stage == 1 {
					wantCalls = 1
				}
				require.Equal(t, wantCalls, calls)
				require.Positive(t, acceptedBytes)
				require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", tc.result)))
				if tc.result == "success" {
					require.Equal(t, float64(out.Size), testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
				} else {
					require.Zero(t, testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "success")))
					require.Zero(t, testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
				}
				assertReleased(t, r)
			})
		})
	}
}

func TestRecorderNativeRetainedBoundary(t *testing.T) {
	for _, delta := range []int64{-1, 0} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.MaxObjectBytes = 1024
			// Allow one maximum-size capture plus its bookkeeping reservation.
			cfg.MaxRetainedBytes = cfg.MaxObjectBytes + itemReservation
			r, _, _ := recorderFixture(t, cfg, discardUpload, nil)
			c := candidate([]byte("owned"))
			item, out := r.prepareCapture("a", c, recorderNow)
			require.Empty(t, out.Reason)
			r.cfg.MaxRetainedBytes = int64(len(c.Payload)+cap(item.metadataJSON)) + itemReservation + delta
			out = r.Capture(context.Background(), "a", c)
			if delta < 0 {
				require.Equal(t, DropByteBudget, out.Reason)
			} else {
				require.True(t, out.Enqueued)
			}
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
			assertReleased(t, r)
		})
	}
}
