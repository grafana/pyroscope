package pyroscope

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockpyroscope"
	"github.com/grafana/pyroscope/v2/pkg/util/tracecontext"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestIngest_PreservesUpstreamTraceContext(t *testing.T) {
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	profile := &profilev1.Profile{
		StringTable: []string{"", "cpu", "nanoseconds"},
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
	}
	raw, err := profile.MarshalVT()
	require.NoError(t, err)

	const callerTraceID = "0123456789abcdef0123456789abcdef"
	for _, format := range []string{"pprof", "lines"} {
		for _, withTrace := range []bool{false, true} {
			name := format + "/without upstream trace"
			if withTrace {
				name = format + "/with upstream trace"
			}
			t.Run(name, func(t *testing.T) {
				svc := mockpyroscope.NewMockPushService(t)
				capture := func(args mock.Arguments) {
					ctx := args.Get(0).(context.Context)
					var want string
					if withTrace {
						want = callerTraceID
					}
					assert.Equal(t, want, tracecontext.UpstreamTraceID(ctx))
					assert.True(t, trace.SpanContextFromContext(ctx).IsValid())
				}
				body := raw
				if format == "pprof" {
					svc.On("PushBatch", mock.Anything, mock.Anything).Run(capture).Return(nil).Once()
				} else {
					body = []byte("foo;bar 1\n")
					svc.On("Push", mock.Anything, mock.Anything).Run(capture).Return(connect.NewResponse(&pushv1.PushResponse{}), nil).Once()
				}
				h := tracecontext.HTTPMiddleware().Wrap(middleware.Tracer{}.Wrap(NewPyroscopeIngestHandler(svc, validation.MockLimits{}, log.NewNopLogger())))
				ctx := tenant.InjectTenantID(t.Context(), "tenant")
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/ingest?name=test&format="+format, bytes.NewReader(body))
				if withTrace {
					req.Header.Set("Traceparent", "00-"+callerTraceID+"-0123456789abcdef-01")
				}
				resp := httptest.NewRecorder()
				h.ServeHTTP(resp, req)
				require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
			})
		}
	}
}
