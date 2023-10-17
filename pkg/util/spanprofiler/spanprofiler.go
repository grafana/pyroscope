package spanprofiler

import (
	"context"
	"runtime/pprof"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
)

const (
	spanIDLabelName   = "span_id"
	spanNameLabelName = "span_name"
)

var profilingEnabledTag = opentracing.Tag{Key: "pyroscope.profiling.enabled", Value: true}

type tracer struct{ opentracing.Tracer }

func NewTracer(tr opentracing.Tracer) opentracing.Tracer { return &tracer{tr} }

func (t *tracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	s := t.Tracer.StartSpan(operationName, opts...)
	if _, ok := parentSpanContextFromRef(opts...); ok {
		return s
	}
	spanID, sampled := getSampledSpanID(s.Context())
	labels := append(make([]string, 0, 4), spanNameLabelName, operationName)
	if sampled {
		labels = append(labels, spanIDLabelName, spanID)
	}
	w := rootSpanWrapper{
		pprofCtx: pprof.WithLabels(context.Background(), pprof.Labels(labels...)),
		Span:     s,
	}
	pprof.SetGoroutineLabels(w.pprofCtx)
	profilingEnabledTag.Set(s)
	return &w
}

func parentSpanContextFromRef(options ...opentracing.StartSpanOption) (opentracing.SpanContext, bool) {
	var sso opentracing.StartSpanOptions
	for _, option := range options {
		option.Apply(&sso)
	}
	for _, ref := range sso.References {
		if ref.Type == opentracing.ChildOfRef && ref.ReferencedContext != nil {
			return ref.ReferencedContext, true
		}
	}
	return nil, false
}

func getSampledSpanID(sc opentracing.SpanContext) (string, bool) {
	if c, ok := sc.(jaeger.SpanContext); ok {
		return c.SpanID().String(), c.IsSampled()
	}
	return "", false
}

type rootSpanWrapper struct {
	pprofCtx context.Context
	opentracing.Span
}

func (s *rootSpanWrapper) Finish() {
	s.Span.Finish()
	pprof.SetGoroutineLabels(context.Background())
}
