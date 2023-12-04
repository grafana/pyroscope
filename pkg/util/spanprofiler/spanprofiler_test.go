package spanprofiler

import (
	"context"
	"runtime/pprof"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

func Test_tracer(t *testing.T) {
	c := &jaegercfg.Configuration{
		ServiceName: "test",
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LocalAgentHostPort: "127.0.0.100:16686",
		},
	}
	tr, closer, err := c.NewTracer()
	if err != nil {
		t.Fatalf("failed to initialize tracer: %v", err)
	}
	defer closer.Close()

	opentracing.SetGlobalTracer(NewTracer(tr))
	const (
		spanIDLabelName   = "span_id"
		spanNameLabelName = "span_name"
	)

	labels := make(map[string]string)

	spanR, ctx := opentracing.StartSpanFromContext(context.Background(), "RootSpan")

	forSpanPprofLabels(spanR, func(key, value string) bool {
		labels[key] = value
		return true
	})
	spanID, ok := labels[spanIDLabelName]
	if !ok {
		t.Fatal("span ID label not found")
	}
	if len(spanID) != 16 {
		t.Fatalf("invalid span ID: %q", spanID)
	}
	name, ok := labels[spanNameLabelName]
	if !ok {
		t.Fatal("span name label not found")
	}
	if name != "RootSpan" {
		t.Fatalf("invalid span name: %q", name)
	}

	// Nested child span has the same labels.
	spanA, ctx := opentracing.StartSpanFromContext(ctx, "SpanA")
	forSpanPprofLabels(spanA, func(key, value string) bool {
		if v, ok := labels[key]; !ok || v != value {
			t.Fatalf("nested span labels mismatch: %q=%q; expected %q=%q", key, value, key, labels[key])
		}
		return true
	})

	spanA.Finish()
	spanR.Finish()

	// Child span created after the root span end using its context.
	spanB, _ := opentracing.StartSpanFromContext(ctx, "SpanB")
	forSpanPprofLabels(spanB, func(key, value string) bool {
		if v, ok := labels[key]; !ok || v != value {
			t.Fatalf("nested span labels mismatch: %q=%q", key, value)
		}
		return true
	})
	spanB.Finish()

	// A new root span.
	spanC, _ := opentracing.StartSpanFromContext(context.Background(), "SpanC")
	forSpanPprofLabels(spanC, func(key, value string) bool {
		if v, ok := labels[key]; !ok || v == value {
			t.Fatalf("unexpected match: %q=%q", key, value)
		}
		return true
	})
	spanC.Finish()
}

func forSpanPprofLabels(span opentracing.Span, fn func(key, value string) bool) {
	w, ok := span.(*spanWrapper)
	if !ok {
		return
	}
	pprof.ForLabels(w.pprofCtx, fn)
}
