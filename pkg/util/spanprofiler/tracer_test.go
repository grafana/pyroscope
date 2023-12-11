package spanprofiler

import (
	"context"
	"io"
	"runtime/pprof"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

func TestTracer_pprof_labels_propagation(t *testing.T) {
	tt := initTestTracer(t)
	defer func() { require.NoError(t, tt.Close()) }()

	t.Run("root span name and ID are propagated as pprof labels", func(t *testing.T) {
		rootSpan, _ := opentracing.StartSpanFromContext(context.Background(), "RootSpan")
		defer rootSpan.Finish()
		pprofLabels := spanPprofLabels(rootSpan)
		// Label / tag names should be specified explicitly.
		require.Equal(t, pprofLabels["span_name"], "RootSpan")
		_, err := jaeger.SpanIDFromString(pprofLabels["span_id"])
		require.NoError(t, err)
		// Make sure the root span has "pyroscope.profile.id" attribute,
		// and it matches the corresponding pprof label.
		require.Equal(t, pprofLabels["span_id"], spanTags(rootSpan)["pyroscope.profile.id"])
	})

	t.Run("pprof labels are propagated to child spans", func(t *testing.T) {
		rootSpan, ctx := opentracing.StartSpanFromContext(context.Background(), "RootSpan")
		defer rootSpan.Finish()
		childSpan, _ := opentracing.StartSpanFromContext(ctx, "ChildSpan")
		defer childSpan.Finish()
		// Goroutine labels are inherited from the parent,
		// we do not set them repeatedly for the child spans.
		require.Empty(t, spanPprofLabels(childSpan))
		// Only the root span is annotated with the profile ID tag.
		require.Nil(t, spanTags(childSpan)["pyroscope.profile.id"])
	})

	t.Run("pprof labels are not propagated to child spans after parent is finished", func(t *testing.T) {
		rootSpan, ctx := opentracing.StartSpanFromContext(context.Background(), "RootSpan")
		rootLabels := spanPprofLabels(rootSpan)
		require.NotEmpty(t, rootLabels)
		// This removes the labels from the goroutine's storage.
		// Note that we can't access them (Go runtime does not provide public
		// methods) but we rely on SetGoroutineLabels implementation:
		// tracer alters currentPprofCtx, so that it actually points to the
		// parentPprofCtx – the state prior StartSpanFromContext call.
		rootSpan.Finish()
		childSpan, _ := opentracing.StartSpanFromContext(ctx, "ChildSpan")
		defer childSpan.Finish()
		childLabels := spanPprofLabels(childSpan)
		require.Empty(t, childLabels)
	})

	t.Run("pprof labels are not propagated to child spans if they are created in a separate goroutine hierarchy", func(t *testing.T) {
		c := make(chan opentracing.SpanContext)
		done := make(chan struct{})
		go func() {
			defer close(done)
			// Normally, we assume that pprof labels are propagated to child
			// goroutines, and we do not have to set pprof labels for each nested
			// span repeatedly. However, if the child span is created in a sibling
			// goroutine, no pprof labels will be attached.
			// The exact span relation (child of / follows from) does not matter.
			span := opentracing.StartSpan("ChildSpan", opentracing.ChildOf(<-c))
			require.Empty(t, spanPprofLabels(span))
			span.Finish()
		}()
		rootSpan, _ := opentracing.StartSpanFromContext(context.Background(), "RootSpan")
		defer rootSpan.Finish()
		c <- rootSpan.Context()
		<-done
	})
}

func initTestTracer(t *testing.T) io.Closer {
	t.Helper()
	// We can't use mock tracer as we actually rely on the
	// Jaeger tracer implementation details.
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
	require.NoError(t, err)
	opentracing.SetGlobalTracer(NewTracer(tr))
	return closer
}

func spanPprofLabels(span opentracing.Span) map[string]string {
	labels := make(map[string]string)
	forSpanPprofLabels(span, func(key, value string) bool {
		labels[key] = value
		return true
	})
	return labels
}

func forSpanPprofLabels(span opentracing.Span, fn func(key, value string) bool) {
	w, ok := span.(*spanWrapper)
	if !ok {
		return
	}
	pprof.ForLabels(w.currentPprofCtx, fn)
}

func spanTags(span opentracing.Span) opentracing.Tags {
	w, ok := span.(*spanWrapper)
	if !ok {
		return nil
	}
	s, ok := w.Span.(*jaeger.Span)
	if !ok {
		return nil
	}
	return s.Tags()
}
