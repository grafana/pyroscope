package spanprofiler

import (
	"context"
	"unsafe"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
)

const (
	profileIDTagKey = "pyroscope.profile.id"

	spanIDLabelName   = "span_id"
	spanNameLabelName = "span_name"
)

type tracer struct{ opentracing.Tracer }

func NewTracer(tr opentracing.Tracer) opentracing.Tracer { return &tracer{tr} }

func (t *tracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	span := t.Tracer.StartSpan(operationName, opts...)
	spanCtx, ok := span.Context().(jaeger.SpanContext)
	if !ok {
		return span
	}
	parent, ok := parentSpanContextFromRef(opts...)
	if ok && !isRemoteSpan(parent) {
		return span
	}
	// The pprof label API assumes that pairs of labels are passed through the
	// context. Unfortunately, the opentracing Tracer API doesn't match this
	// concept. This makes it impossible to save an existing pprof context and
	// all the original pprof labels associated with the goroutine.
	return wrapJaegerSpanWithGoroutineLabels(context.Background(), span, operationName, sampledSpanID(spanCtx))
}

func parentSpanContextFromRef(options ...opentracing.StartSpanOption) (jaeger.SpanContext, bool) {
	var sso opentracing.StartSpanOptions
	for _, option := range options {
		option.Apply(&sso)
	}
	for _, ref := range sso.References {
		if ref.Type == opentracing.ChildOfRef && ref.ReferencedContext != nil {
			c, ok := ref.ReferencedContext.(jaeger.SpanContext)
			return c, ok
		}
	}
	return jaeger.SpanContext{}, false
}

func isRemoteSpan(c jaeger.SpanContext) bool {
	// This is ugly and unsafe, but is the only reliable way to get to know which
	// spans should be profiled. The opentracing-go package and Jaeger client
	// are not meant to change as both are deprecated.
	defer func() { recover() }()
	jaegerCtx := *(*jaegerSpanCtx)(unsafe.Pointer(&c))
	return jaegerCtx.remote
}

type jaegerSpanCtx struct {
	traceID       [16]byte   // TraceID
	spanID        [8]byte    // SpanID
	parentID      [8]byte    // SpanID
	baggage       uintptr    // map[string]string
	debugID       [2]uintptr // string
	samplingState uintptr
	// remote indicates that span context represents a remote parent
	remote bool
}
