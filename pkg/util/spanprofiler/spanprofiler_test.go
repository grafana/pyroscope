package spanprofiler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go"
)

func TestSpanProfiler_pprof_labels_propagation(t *testing.T) {
	tt := initTestTracer(t)
	defer func() { require.NoError(t, tt.Close()) }()

	t.Run("pprof labels are not propagated to child spans", func(t *testing.T) {
		rootSpan, ctx := StartSpanFromContext(context.Background(), "RootSpan")
		defer rootSpan.Finish()
		rootLabels := spanPprofLabels(rootSpan)
		require.Equal(t, rootLabels["span_name"], "RootSpan")
		rootSpanID, err := jaeger.SpanIDFromString(rootLabels["span_id"])
		require.NoError(t, err)

		// Regardless of anything, pprof labels are attached to the current
		// goroutine, and the "pyroscope.profile.id" tag is set.
		childSpan, _ := StartSpanFromContext(ctx, "ChildSpan")
		defer childSpan.Finish()
		childLabels := spanPprofLabels(childSpan)
		require.Equal(t, childLabels["span_name"], "ChildSpan")
		childSpanID, err := jaeger.SpanIDFromString(childLabels["span_id"])
		require.NoError(t, err)

		require.NotEqual(t, rootSpanID, childSpanID)
		require.Equal(t, childSpanID.String(), spanTags(childSpan)["pyroscope.profile.id"])
	})
}
