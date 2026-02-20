package tracing

import (
	"context"

	dskittracing "github.com/grafana/dskit/tracing"
)

// StartSpanFromContext starts a new span from context. If no tracer is
// registered, a noop span is returned.
func StartSpanFromContext(ctx context.Context, operationName string) (*dskittracing.Span, context.Context) {
	return dskittracing.StartSpanFromContext(ctx, operationName)
}
