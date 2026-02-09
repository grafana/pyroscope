package timeseriescompact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

func TestRangeSeries_EmptyIterator(t *testing.T) {
	it := iter.NewEmptyIterator[CompactValue]()
	series := RangeSeries(it, 0, 1000, 100)
	assert.Nil(t, series)
}

func TestRangeSeries_SinglePoint(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points:        []*queryv1.Point{{Timestamp: 500, Value: 100}},
		}},
	})

	series := RangeSeries(m.Iterator(), 0, 1000, 1000)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.Equal(t, int64(1000), series[0].Points[0].Timestamp)
	assert.Equal(t, 100.0, series[0].Points[0].Value)
}

func TestRangeSeries_AggregatesWithinStep(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 100, Value: 10},
				{Timestamp: 200, Value: 20},
				{Timestamp: 300, Value: 30},
			},
		}},
	})

	// Step of 1000 should aggregate all points into one
	series := RangeSeries(m.Iterator(), 0, 1000, 1000)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.Equal(t, 60.0, series[0].Points[0].Value)
}

func TestRangeSeries_MultipleSteps(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 100, Value: 10},
				{Timestamp: 500, Value: 50},
				{Timestamp: 1500, Value: 150},
				{Timestamp: 2500, Value: 250},
			},
		}},
	})

	series := RangeSeries(m.Iterator(), 0, 3000, 1000)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 3)

	// First step (0-1000): 10 + 50 = 60
	assert.Equal(t, int64(1000), series[0].Points[0].Timestamp)
	assert.Equal(t, 60.0, series[0].Points[0].Value)

	// Second step (1000-2000): 150
	assert.Equal(t, int64(2000), series[0].Points[1].Timestamp)
	assert.Equal(t, 150.0, series[0].Points[1].Value)

	// Third step (2000-3000): 250
	assert.Equal(t, int64(3000), series[0].Points[2].Timestamp)
	assert.Equal(t, 250.0, series[0].Points[2].Value)
}

func TestRangeSeries_MultipleSeries(t *testing.T) {
	m := NewMerger()

	// Series 1: service=api
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points:        []*queryv1.Point{{Timestamp: 500, Value: 100}},
		}},
	})

	// Series 2: service=web
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"web"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points:        []*queryv1.Point{{Timestamp: 500, Value: 200}},
		}},
	})

	series := RangeSeries(m.Iterator(), 0, 1000, 1000)
	require.Len(t, series, 2)

	// Each series should have one point
	for _, s := range series {
		require.Len(t, s.Points, 1)
	}

	// Total value across both series
	total := series[0].Points[0].Value + series[1].Points[0].Value
	assert.Equal(t, 300.0, total)
}

func TestRangeSeries_WithAnnotations(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service", "error", "host"},
			Values: []string{"api", "true", "server-1"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 100, Value: 10, AnnotationRefs: []int64{1}},
				{Timestamp: 200, Value: 20, AnnotationRefs: []int64{2}},
			},
		}},
	})

	series := RangeSeries(m.Iterator(), 0, 1000, 1000)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)

	// Should have both annotations merged and deduped
	point := series[0].Points[0]
	assert.Len(t, point.AnnotationRefs, 2)
}

func TestRangeSeries_WithExemplars(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service", "pod"},
			Values: []string{"api", "a"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{
					Timestamp: 100,
					Value:     100,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-1",
						Value:         100,
						AttributeRefs: []int64{1},
					}},
				},
				{
					Timestamp: 200,
					Value:     50,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-2",
						Value:         50,
						AttributeRefs: []int64{1},
					}},
				},
			},
		}},
	})

	series := RangeSeries(m.Iterator(), 0, 1000, 1000)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)

	point := series[0].Points[0]
	assert.Equal(t, 150.0, point.Value) // 100 + 50

	// Should have top exemplar (highest value)
	require.Len(t, point.Exemplars, 1)
	assert.Equal(t, "prof-1", point.Exemplars[0].ProfileId)
}

func TestRangeSeries_DifferentTimestampsNoCorruption(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service", "a", "b", "c"},
			Values: []string{"api", "1", "2", "3"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 100, AnnotationRefs: []int64{1}},
				{Timestamp: 2000, Value: 200, AnnotationRefs: []int64{2}},
				{Timestamp: 3000, Value: 300, AnnotationRefs: []int64{3}},
			},
		}},
	})

	series := RangeSeries(m.Iterator(), 1000, 3000, 1000)
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 3)

	// Each point should have its own annotation, not corrupted
	assert.Equal(t, []int64{1}, series[0].Points[0].AnnotationRefs)
	assert.Equal(t, []int64{2}, series[0].Points[1].AnnotationRefs)
	assert.Equal(t, []int64{3}, series[0].Points[2].AnnotationRefs)
}
