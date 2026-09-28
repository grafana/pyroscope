// Package tracecontext preserves the caller's trace identity independently of
// spans created while processing the request.
package tracecontext

import (
	"context"
	"net/http"

	"github.com/grafana/dskit/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type upstreamTraceIDKey struct{}

// UpstreamTraceID returns the trace ID extracted at ingress, or an empty string
// if the caller did not supply a valid trace. It never falls back to local spans.
func UpstreamTraceID(ctx context.Context) string {
	id, _ := ctx.Value(upstreamTraceIDKey{}).(string)
	return id
}

// HTTPMiddleware captures the caller's trace headers before request processing.
func HTTPMiddleware() middleware.Interface {
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := capture(r.Context(), propagation.HeaderCarrier(r.Header))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}

// UnaryServerInterceptor captures the caller's trace from incoming gRPC metadata.
// Extracting independently of the active span also handles tracing stats handlers,
// which run before unary interceptors and may already have created a local trace.
func UnaryServerInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	return handler(capture(ctx, metadataCarrier(md)), req)
}

func capture(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	// A propagator leaves the input context unchanged when trace headers are
	// absent or invalid. Use a clean context so it cannot inherit a local trace.
	parent := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	var id string
	if sc := trace.SpanContextFromContext(parent); sc.IsValid() {
		id = sc.TraceID().String()
	}
	return context.WithValue(ctx, upstreamTraceIDKey{}, id)
}

type metadataCarrier metadata.MD

func (c metadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (c metadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(key, value)
}

func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}
