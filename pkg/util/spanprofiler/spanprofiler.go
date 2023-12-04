package spanprofiler

import (
	"context"
	"runtime/pprof"
	"sync"
	"unsafe"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"go.uber.org/atomic"
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
	sc, ok := s.Context().(jaeger.SpanContext)
	if !ok || !isLocalRoot(sc) {
		return s
	}
	// Note that pprof labels are propagated through the goroutine's local
	// storage and are always copied to child goroutines. This way, stack
	// trace samples collected during execution of child spans will be taken
	// into account at the root.
	var labels []string
	if sc.IsSampled() {
		labels = []string{spanNameLabelName, operationName, spanIDLabelName, sc.SpanID().String()}
	} else {
		// Even if the trace has not been sampled, we still need to keep track
		// of samples that belong to the span (all spans with the given name).
		labels = []string{spanNameLabelName, operationName}
	}
	// The pprof label API assumes that pairs of labels are passed through the
	// context. Unfortunately, the opentracing Tracer API doesn't match this
	// idea. This makes it impossible to save an existing pprof context and all
	// the original pprof labels.
	ctx := context.Background()
	// We create a span wrapper to ensure we remove the newly attached pprof
	// labels when span finishes. The need of this wrapper is questioned:
	// as we do not have the original context, we could leave the goroutine
	// labels – normally, span is finished at the very end of the goroutine's
	// lifetime, so no significant side effects should take place.
	pprofCtx := pprof.WithLabels(ctx, pprof.Labels(labels...))
	w := spanWrapper{pprofCtx: pprofCtx, Span: s}
	pprof.SetGoroutineLabels(w.pprofCtx)
	opentracing.Tag{Key: profileIDTagKey, Value: sc.SpanID().String()}.Set(s)
	return &w
}

func isLocalRoot(c jaeger.SpanContext) bool {
	// This is ugly and unsafe, but the only reliable way to get to know which
	// spans should be profiled. The opentracing-go package and Jaeger client
	// are not meant to change as both are deprecated.
	defer func() { recover() }()
	jaegerCtx := *(*jaegerSpanCtx)(unsafe.Pointer(&c))
	if jaegerCtx.samplingState != nil {
		return jaegerCtx.samplingState.localRootSpan == jaegerCtx.spanID
	}
	return false
}

type jaegerSpanCtx struct {
	traceID       [16]byte   // TraceID
	spanID        [8]byte    // SpanID
	parentID      [8]byte    // SpanID
	baggage       uintptr    // map[string]string
	debugID       [2]uintptr // string
	samplingState *samplingState
	remote        bool
}

type samplingState struct {
	stateFlags    atomic.Int32
	final         atomic.Bool
	localRootSpan [8]byte
	extendedState sync.Map
}

type spanWrapper struct {
	pprofCtx context.Context
	opentracing.Span
}

func (s *spanWrapper) Finish() {
	s.Span.Finish()
	pprof.SetGoroutineLabels(context.Background())
}
