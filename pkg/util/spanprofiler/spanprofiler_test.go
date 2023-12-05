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
		spanR, ctx := StartSpanFromContext(context.Background(), "RootSpan")
		defer spanR.Finish()
		rootLabels := spanPprofLabels(spanR)
		require.Equal(t, rootLabels["span_name"], "RootSpan")
		rootSpanID, err := jaeger.SpanIDFromString(rootLabels["span_id"])
		require.NoError(t, err)

		// Regardless of anything, pprof labels are attached to the current
		// goroutine, and the "pyroscope.profile.id" tag is set.
		spanA, _ := StartSpanFromContext(ctx, "ChildSpan")
		defer spanA.Finish()
		childLabels := spanPprofLabels(spanA)
		require.Equal(t, childLabels["span_name"], "ChildSpan")
		childSpanID, err := jaeger.SpanIDFromString(childLabels["span_id"])
		require.NoError(t, err)

		require.NotEqual(t, rootSpanID, childSpanID)
		require.Equal(t, childLabels["span_id"], spanTags(spanA)["pyroscope.profile.id"])
	})
}
