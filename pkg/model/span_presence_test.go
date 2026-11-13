package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestSpanPresenceMerger(t *testing.T) {
	var m SpanPresenceMerger
	entry := func(trace, typ string, total int64) *querierv1.SpanPresence {
		return &querierv1.SpanPresence{TraceId: trace, SpanId: "01", Totals: []*querierv1.SpanProfileTotal{{ProfileTypeID: typ, Total: total}}}
	}
	m.Merge([]*querierv1.SpanPresence{entry("a", "cpu", 3), entry("a", "cpu", 4), entry("a", "alloc", 0), entry("b", "cpu", 2), entry("", "cpu", 1)})
	result := m.Spans()
	require.Len(t, result, 3)
	require.Equal(t, "", result[0].TraceId)
	require.Equal(t, "a", result[1].TraceId)
	require.Equal(t, []*querierv1.SpanProfileTotal{{ProfileTypeID: "alloc", Total: 0}, {ProfileTypeID: "cpu", Total: 7}}, result[1].Totals)
	// Results are snapshots, not aliases of aggregation state.
	result[1].Totals[1].Total = 999
	require.EqualValues(t, 7, m.Spans()[1].Totals[1].Total)
}
