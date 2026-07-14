package heatmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestMerger_DoesNotMergeDifferentTraceIDs(t *testing.T) {
	traceID1 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	traceID2 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	report := &queryv1.HeatmapReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{""},
			Values: []string{"profile-id"},
		},
		HeatmapSeries: []*queryv1.HeatmapSeries{{
			Points: []*queryv1.HeatmapPoint{
				{Timestamp: 100, ProfileId: 0, SpanId: 123, TraceId: traceID1, Value: 1000},
				{Timestamp: 100, ProfileId: 0, SpanId: 123, TraceId: traceID2, Value: 2000},
			},
		}},
	}

	merger := NewMerger(true)
	merger.MergeHeatmap(report)
	result := merger.Build()

	require.Len(t, result.HeatmapSeries, 1)
	require.Len(t, result.HeatmapSeries[0].Points, 2)
	assert.Equal(t, traceID1, result.HeatmapSeries[0].Points[0].TraceId)
	assert.Equal(t, traceID2, result.HeatmapSeries[0].Points[1].TraceId)
}

func TestMerger_MergesMatchingTraceIDs(t *testing.T) {
	traceID := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	report := &queryv1.HeatmapReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{""},
			Values: []string{"profile-id"},
		},
		HeatmapSeries: []*queryv1.HeatmapSeries{{
			Points: []*queryv1.HeatmapPoint{
				{Timestamp: 100, ProfileId: 0, SpanId: 123, TraceId: traceID, Value: 1000},
				{Timestamp: 100, ProfileId: 0, SpanId: 123, TraceId: traceID, Value: 2000},
			},
		}},
	}

	merger := NewMerger(true)
	merger.MergeHeatmap(report)
	result := merger.Build()

	require.Len(t, result.HeatmapSeries, 1)
	require.Len(t, result.HeatmapSeries[0].Points, 1)
	assert.Equal(t, int64(3000), result.HeatmapSeries[0].Points[0].Value)
}
