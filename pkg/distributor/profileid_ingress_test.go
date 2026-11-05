package distributor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model/profileid"
	pprof2 "github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/util/tracecontext"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestPush_ProfileIDsWithServerTracing(t *testing.T) {
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	const traceID = "0123456789abcdef0123456789abcdef"
	const traceparent = "00-" + traceID + "-0123456789abcdef-01"
	const suppliedID = "01234567-89ab-4def-8123-456789abcdef"
	parsed, err := pprof2.RawFromBytes(collectTestProfileBytes(t))
	require.NoError(t, err)
	labels := []*typesv1.LabelPair{
		{Name: "__name__", Value: "cpu"},
		{Name: "service_name", Value: "service"},
	}

	for _, protocol := range []string{"connect", "grpc"} {
		for _, tt := range []struct {
			name        string
			timestamp   int64
			traceparent string
			id          string
			disabled    bool
			source      profileid.Source
		}{
			{name: "random without upstream trace", source: profileid.SourceRandom},
			{name: "random with invalid trace", traceparent: "invalid", source: profileid.SourceRandom},
			{name: "upstream trace", traceparent: traceparent, source: profileid.SourceTraceID},
			{name: "timestamp takes precedence", timestamp: time.Now().UnixNano(), traceparent: traceparent, source: profileid.SourceTimestamp},
			{name: "user UUID takes precedence", timestamp: time.Now().UnixNano(), traceparent: traceparent, id: suppliedID, source: profileid.SourceUserSupplied},
			{name: "invalid UUID with timestamp", timestamp: time.Now().UnixNano(), traceparent: traceparent, id: "not-a-uuid", source: profileid.SourceTimestamp},
			{name: "invalid UUID with trace", traceparent: traceparent, id: "not-a-uuid", source: profileid.SourceTraceID},
			{name: "invalid UUID without timestamp or trace", id: "not-a-uuid", source: profileid.SourceRandom},
			{name: "invalid UUID with timestamp disabled", timestamp: time.Now().UnixNano(), traceparent: traceparent, id: "not-a-uuid", disabled: true, source: profileid.SourceRandom},
			{name: "invalid UUID with trace disabled", traceparent: traceparent, id: "not-a-uuid", disabled: true, source: profileid.SourceRandom},
			{name: "invalid UUID without timestamp or trace disabled", id: "not-a-uuid", disabled: true, source: profileid.SourceRandom},
			{name: "user UUID preserved when disabled", timestamp: time.Now().UnixNano(), traceparent: traceparent, id: suppliedID, disabled: true, source: profileid.SourceUserSupplied},
		} {
			t.Run(protocol+"/"+tt.name, func(t *testing.T) {
				d, ing, err := newTestDistributor(t, log.NewNopLogger(), validation.MockOverrides(func(l *validation.Limits, _ map[string]*validation.Limits) {
					l.ProfileIDDeterministic = !tt.disabled
				}))
				require.NoError(t, err)
				mux := http.NewServeMux()
				mux.Handle(pushv1connect.NewPusherServiceHandler(d, connect.WithInterceptors(tenant.NewAuthInterceptor(true))))
				server := httptest.NewUnstartedServer(tracecontext.HTTPMiddleware().Wrap(middleware.Tracer{}.Wrap(mux)))
				server.EnableHTTP2 = true
				server.StartTLS()
				t.Cleanup(server.Close)
				opts := []connect.ClientOption{connect.WithInterceptors(tenant.NewAuthInterceptor(true))}
				if protocol == "grpc" {
					opts = append(opts, connect.WithGRPC())
				}
				client := pushv1connect.NewPusherServiceClient(server.Client(), server.URL, opts...)
				profile := parsed.CloneVT()
				profile.TimeNanos = tt.timestamp
				raw, err := profile.MarshalVT()
				require.NoError(t, err)

				// Exercise both multiple profiles in a request and retries. Locally
				// generated traces must not make independent profiles share an ID.
				for range 2 {
					req := connect.NewRequest(&pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
						Labels: labels,
						Samples: []*pushv1.RawSample{
							{RawProfile: raw, ID: tt.id},
							{RawProfile: raw, ID: tt.id},
						},
					}}})
					req.Header().Set("Traceparent", tt.traceparent)
					_, err = client.Push(tenant.InjectTenantID(t.Context(), "tenant"), req)
					require.NoError(t, err)
				}
				ids := make(map[string]struct{})
				for _, request := range ing.requests {
					for _, series := range request.Series {
						for _, sample := range series.Samples {
							ids[sample.ID] = struct{}{}
						}
					}
				}
				switch tt.source {
				case profileid.SourceRandom:
					assert.Len(t, ids, 4)
				case profileid.SourceUserSupplied:
					assert.Equal(t, map[string]struct{}{suppliedID: {}}, ids)
				default:
					expected, _ := profileid.Generate("tenant", "cpu", labels, tt.timestamp, traceID)
					assert.Equal(t, map[string]struct{}{expected.String(): {}}, ids)
				}
				for _, source := range []profileid.Source{profileid.SourceRandom, profileid.SourceTraceID, profileid.SourceTimestamp, profileid.SourceUserSupplied} {
					var want float64
					if source == tt.source && !tt.disabled {
						want = 4
					}
					assert.Equal(t, want, testutil.ToFloat64(d.metrics.profileIDGeneration.WithLabelValues(string(source))))
				}
			})
		}
	}
}
