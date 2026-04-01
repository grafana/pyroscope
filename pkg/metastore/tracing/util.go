package tracing

import (
	"context"

	dskittracing "github.com/grafana/dskit/tracing"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// StartSpanFromContext starts a new span from context only if the context
// already carries a parent span. This avoids creating orphaned root spans
// in places like metastore followers where no trace context is available.
func StartSpanFromContext(ctx context.Context, operationName string) (*dskittracing.Span, context.Context) {
	if !oteltrace.SpanFromContext(ctx).SpanContext().IsValid() {
		return &dskittracing.Span{}, ctx
	}
	return dskittracing.StartSpanFromContext(ctx, operationName)
}
