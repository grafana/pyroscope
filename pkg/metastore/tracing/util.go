package tracing

import (
	"context"

	"github.com/opentracing/opentracing-go"
)

var noopTracer = opentracing.NoopTracer{}

// StartSpanFromContext starts a span only if there's a parent span in the context.
// Otherwise, it returns a noop span. To be used in places where we might not have access to the original context.
func StartSpanFromContext(ctx context.Context, operationName string) (opentracing.Span, context.Context) {
	if opentracing.SpanFromContext(ctx) != nil {
		return opentracing.StartSpanFromContext(ctx, operationName)
	}
	return noopTracer.StartSpan(operationName), ctx
}
