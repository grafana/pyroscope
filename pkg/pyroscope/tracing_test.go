package pyroscope

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/grafana/pyroscope/v2/pkg/util/tracecontext"
)

func TestTracePropagationWithoutExporter(t *testing.T) {
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
	})
	const traceID = "0123456789abcdef0123456789abcdef"
	for _, tt := range []struct {
		name        string
		propagators string
		header      string
		value       string
		want        string
	}{
		{name: "default W3C", header: "Traceparent", value: "00-" + traceID + "-0123456789abcdef-01", want: traceID},
		{name: "default Jaeger", header: "Uber-Trace-Id", value: traceID + ":0123456789abcdef:0:1", want: traceID},
		{name: "configured Jaeger", propagators: "jaeger", header: "Uber-Trace-Id", value: traceID + ":0123456789abcdef:0:1", want: traceID},
		{name: "disabled propagation", propagators: "none", header: "Traceparent", value: "00-" + traceID + "-0123456789abcdef-01"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_PROPAGATORS", tt.propagators)
			initTracePropagation()
			req := httptest.NewRequest(http.MethodPost, "/ingest", nil)
			req.Header.Set(tt.header, tt.value)
			h := tracecontext.HTTPMiddleware().Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.want, tracecontext.UpstreamTraceID(r.Context()))
				assert.False(t, trace.SpanContextFromContext(r.Context()).IsValid(), "capturing the caller ID must not start server tracing")
			}))
			h.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
