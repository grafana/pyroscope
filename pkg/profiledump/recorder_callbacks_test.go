package profiledump

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecorderCallbackPanics(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		stage                  string
		finishPanic, sizeError bool
		reason                 DropReason
	}{
		{name: "sizing", stage: "size", reason: DropPanic},
		{name: "serialization", stage: "write", reason: DropPanic},
		{name: "span start", stage: "start", reason: DropPanic},
		{name: "span finish after enqueue", finishPanic: true},
		{name: "span finish after error", finishPanic: true, sizeError: true, reason: DropSerialization},
		{name: "span finish after panic", stage: "write", finishPanic: true, reason: DropPanic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			var uploads atomic.Int64
			r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, tenant, key string, body io.Reader) error {
				uploads.Add(1)
				return discardUpload(ctx, tenant, key, body)
			}, nil)
			const panicValue = "request data must not become a metric label"
			p := &funcPayload{size: 4, write: func(_ context.Context, dst, _ []byte) (int, error) {
				n := copy(dst, "data")
				if tc.stage == "write" {
					panic(panicValue)
				}
				return n, nil
			}}
			if tc.stage == "size" {
				p.sizeFunc = func() (int64, int64, error) { panic(panicValue) }
			}
			if tc.sizeError {
				p.sizeErr = errors.New("sizing failed")
			}
			c := candidate(nil)
			c.Payload = p
			spanStart := CaptureSpan(provider.Tracer("test"))
			var finishes int
			c.StartSpan = func(ctx context.Context) (context.Context, func(Outcome)) {
				if tc.stage == "start" {
					panic(panicValue)
				}
				ctx, finish := spanStart(ctx)
				return ctx, func(out Outcome) {
					finishes++
					finish(out)
					if tc.finishPanic {
						panic(panicValue)
					}
				}
			}
			var out Outcome
			require.NotPanics(t, func() { out = r.Capture(context.Background(), "a", c) })
			require.Equal(t, tc.reason, out.Reason)
			require.Equal(t, tc.reason == "", out.Enqueued)
			if tc.stage == "start" {
				require.Zero(t, p.sizes.Load())
				require.Zero(t, finishes)
				require.Empty(t, exporter.GetSpans())
			} else {
				require.Equal(t, 1, finishes)
				spans := exporter.GetSpans()
				require.Len(t, spans, 1)
				require.Equal(t, out.SpanAttributes(), spans[0].Attributes, "span must see the final panic/drop/enqueue outcome")
			}
			if !out.Enqueued {
				assertReleased(t, r)
				require.Zero(t, uploads.Load())
				require.Equal(t, 1., testutil.ToFloat64(r.metrics.dropped.WithLabelValues("connect", string(tc.reason))))
				require.Equal(t, 1., testutil.ToFloat64(r.metrics.candidates.WithLabelValues("connect", "dropped")))
				require.Equal(t, float64(out.Size), testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "dropped")))
			} else {
				require.Equal(t, 1., testutil.ToFloat64(r.metrics.candidates.WithLabelValues("connect", "enqueued")))
				require.Equal(t, float64(out.Size), testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "enqueued")))
			}
			require.Zero(t, testutil.ToFloat64(r.metrics.dropped.WithLabelValues("connect", "")))

			// The failed callback must not leave a lock held or poison subsequent work.
			require.True(t, r.Capture(context.Background(), "a", candidate([]byte("next"))).Enqueued)
			require.NoError(t, r.Shutdown(context.Background()))
			wantUploads := int64(1)
			if out.Enqueued {
				wantUploads++
			}
			require.Equal(t, wantUploads, uploads.Load())
			assertReleased(t, r)
		})
	}
}

func TestRecorderPayloadWriteCount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		size     int64
		body     string
		reported int
		err      error
		enqueued bool
	}{
		{name: "short", size: 8, body: "short", reported: 5},
		{name: "no bytes", size: 8},
		{name: "negative count", size: 8, reported: -1},
		{name: "oversized count", size: 8, body: "complete", reported: 9},
		{name: "full count with error", size: 8, body: "complete", reported: 8, err: io.ErrUnexpectedEOF},
		{name: "complete", size: 8, body: "complete", reported: 8, enqueued: true},
		{name: "empty", enqueued: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uploaded := make(chan []byte, 1)
			r, _, _ := recorderFixture(t, recorderTestConfig(), func(_ context.Context, _, _ string, body io.Reader) error {
				data, err := io.ReadAll(body)
				uploaded <- data
				return err
			}, nil)
			c := candidate(nil)
			c.Payload = &funcPayload{size: tc.size, scratch: 16, write: func(_ context.Context, dst, _ []byte) (int, error) {
				copy(dst, tc.body)
				return tc.reported, tc.err
			}}
			var traced Outcome
			c.StartSpan = func(ctx context.Context) (context.Context, func(Outcome)) {
				return ctx, func(out Outcome) { traced = out }
			}
			out := r.Capture(context.Background(), "a", c)
			require.Equal(t, tc.enqueued, out.Enqueued)
			require.Equal(t, out, traced)
			if !tc.enqueued {
				require.Equal(t, DropSerialization, out.Reason)
				assertReleased(t, r)
				require.Equal(t, 1., testutil.ToFloat64(r.metrics.dropped.WithLabelValues("connect", string(DropSerialization))))
			}
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
			if tc.enqueued {
				var payload bytes.Buffer
				_, err := Decode(bytes.NewReader(await(t, uploaded)), &payload, r.cfg.MaxObjectBytes)
				require.NoError(t, err)
				require.Equal(t, tc.body, payload.String())
			} else {
				require.Empty(t, uploaded, "an incomplete capture must never reach storage")
			}
		})
	}
}

func TestBytesPayloadWriteCount(t *testing.T) {
	p := BytesPayload("native")
	dst := make([]byte, len(p)-1)
	n, err := p.Write(context.Background(), dst, nil)
	require.Equal(t, len(dst), n)
	require.ErrorIs(t, err, io.ErrShortWrite)

	dst = make([]byte, len(p)+1)
	n, err = p.Write(context.Background(), dst, nil)
	require.Equal(t, len(p), n, "report copied bytes, not destination length")
	require.NoError(t, err)
	require.Equal(t, []byte(p), dst[:n])

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err = p.Write(ctx, dst, nil)
	require.Zero(t, n)
	require.ErrorIs(t, err, context.Canceled)
}
