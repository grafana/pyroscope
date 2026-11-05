package tracecontext

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/dskit/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

const callerTraceID = "0123456789abcdef0123456789abcdef"

var traceCases = []struct {
	name        string
	traceparent string
	want        string
}{
	{name: "no upstream trace"},
	{name: "sampled", traceparent: "00-" + callerTraceID + "-0123456789abcdef-01", want: callerTraceID},
	{name: "unsampled", traceparent: "00-" + callerTraceID + "-0123456789abcdef-00", want: callerTraceID},
	{name: "invalid", traceparent: "not-a-trace"},
	{name: "zero trace ID", traceparent: "00-00000000000000000000000000000000-0123456789abcdef-01"},
}

func setupTracing(t *testing.T) {
	t.Helper()
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
}

func TestHTTPMiddleware(t *testing.T) {
	setupTracing(t)
	for _, tt := range traceCases {
		t.Run(tt.name, func(t *testing.T) {
			// An existing local span must not be mistaken for a caller trace,
			// even when propagation cannot extract a valid parent.
			ctx, localSpan := otel.Tracer("test").Start(t.Context(), "local")
			defer localSpan.End()
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/ingest", nil)
			req.Header.Set("Traceparent", tt.traceparent)
			called := false
			h := HTTPMiddleware().Wrap(middleware.Tracer{}.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				assert.True(t, trace.SpanContextFromContext(r.Context()).IsValid(), "server tracing must remain active")
				assert.Equal(t, tt.want, UpstreamTraceID(r.Context()))
				childCtx, span := otel.Tracer("test").Start(r.Context(), "ingestion")
				defer span.End()
				assert.Equal(t, tt.want, UpstreamTraceID(childCtx))
			})))
			h.ServeHTTP(httptest.NewRecorder(), req)
			require.True(t, called)
		})
	}
}

func TestUpstreamTraceIDDoesNotFallBackToActiveSpan(t *testing.T) {
	setupTracing(t)
	ctx, span := otel.Tracer("test").Start(t.Context(), "local")
	defer span.End()
	require.True(t, trace.SpanContextFromContext(ctx).IsValid())
	assert.Empty(t, UpstreamTraceID(ctx))
}

type capturedTrace struct {
	upstream string
	active   trace.SpanContext
}

type traceHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	captured chan capturedTrace
}

func (s *traceHealthServer) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	ctx, span := otel.Tracer("test").Start(ctx, "ingestion")
	defer span.End()
	s.captured <- capturedTrace{upstream: UpstreamTraceID(ctx), active: trace.SpanContextFromContext(ctx)}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func TestUnaryServerInterceptor(t *testing.T) {
	setupTracing(t)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(UnaryServerInterceptor),
	)
	service := &traceHealthServer{captured: make(chan capturedTrace, 1)}
	grpc_health_v1.RegisterHealthServer(server, service)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-serveErr; err != nil {
			require.ErrorIs(t, err, grpc.ErrServerStopped)
		}
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	client := grpc_health_v1.NewHealthClient(conn)
	for _, tt := range traceCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if tt.traceparent != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", tt.traceparent)
			}
			_, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
			require.NoError(t, err)
			got := <-service.captured
			assert.True(t, got.active.IsValid(), "gRPC stats handler must still create a server trace")
			assert.Equal(t, tt.want, got.upstream)
			if tt.want != "" {
				assert.Equal(t, tt.want, got.active.TraceID().String())
			}
		})
	}
}
