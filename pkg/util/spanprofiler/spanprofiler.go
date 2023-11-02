package spanprofiler

import (
	"context"
	"runtime/pprof"
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
	s := t.Tracer.StartSpan(operationName, opts...)
	parent, ok := parentSpanContextFromRef(opts...)
	if ok && !isRemoteSpan(parent) {
		return s
	}
	sc, ok := s.Context().(jaeger.SpanContext)
	if !ok {
		return s
	}
	labels := append(make([]string, 0, 4), spanNameLabelName, operationName)
	if sc.IsSampled() {
		labels = append(labels, spanIDLabelName, sc.SpanID().String())
	}
	w := rootSpanWrapper{
		pprofCtx: pprof.WithLabels(context.Background(), pprof.Labels(labels...)),
		Span:     s,
	}
	pprof.SetGoroutineLabels(w.pprofCtx)
	tag := opentracing.Tag{Key: profileIDTagKey, Value: sc.SpanID().String()}
	tag.Set(s)
	return &w
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
	jaegerCtx := *(*jaegerSpanCtx)(unsafe.Pointer(&c))
	return jaegerCtx.remote
}

type jaegerSpanCtx struct {
	traceID       [16]byte // TraceID
	spanID        [8]byte  // SpanID
	parentID      [8]byte  // SpanID
	baggage       uint     // map[string]string
	debugID       [2]uint  // string
	samplingState uint     // *samplingState
	// remote indicates that span context represents a remote parent
	remote bool
}

type rootSpanWrapper struct {
	pprofCtx context.Context
	opentracing.Span
}

func (s *rootSpanWrapper) Finish() {
	s.Span.Finish()
	pprof.SetGoroutineLabels(context.Background())
}
