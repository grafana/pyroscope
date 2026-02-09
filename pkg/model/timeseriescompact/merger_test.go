package timeseriescompact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestMerger_EmptyReport(t *testing.T) {
	m := NewMerger()
	m.MergeReport(nil)
	m.MergeReport(&queryv1.TimeSeriesCompactReport{})

	it := m.Iterator()
	assert.False(t, it.Next())
}

func TestMerger_SingleReport(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 100},
				{Timestamp: 2000, Value: 200},
			},
		}},
	})

	it := m.Iterator()
	require.True(t, it.Next())
	v := it.At()
	assert.Equal(t, int64(1000), v.Ts)
	assert.Equal(t, 100.0, v.Value)

	require.True(t, it.Next())
	v = it.At()
	assert.Equal(t, int64(2000), v.Ts)
	assert.Equal(t, 200.0, v.Value)

	assert.False(t, it.Next())
}

func TestMerger_MultipleReports_SameSeries(t *testing.T) {
	m := NewMerger()

	// Report 1
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 100},
			},
		}},
	})

	// Report 2 - same series, different timestamp
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 2000, Value: 200},
			},
		}},
	})

	it := m.Iterator()
	var points []CompactValue
	for it.Next() {
		points = append(points, it.At())
	}

	require.Len(t, points, 2)
	assert.Equal(t, int64(1000), points[0].Ts)
	assert.Equal(t, int64(2000), points[1].Ts)
}

func TestMerger_MultipleReports_DifferentSeries(t *testing.T) {
	m := NewMerger()

	// Report 1 - service=api
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 100},
			},
		}},
	})

	// Report 2 - service=web
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service"},
			Values: []string{"web"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 200},
			},
		}},
	})

	it := m.Iterator()
	var points []CompactValue
	for it.Next() {
		points = append(points, it.At())
	}

	// Two different series, both at timestamp 1000
	require.Len(t, points, 2)
	// Both should have same timestamp (merge iterator returns in timestamp order)
	assert.Equal(t, int64(1000), points[0].Ts)
	assert.Equal(t, int64(1000), points[1].Ts)
	// Different series keys
	assert.NotEqual(t, points[0].SeriesKey, points[1].SeriesKey)
}

func TestMerger_RemapsAttributeRefs(t *testing.T) {
	m := NewMerger()

	// Report 1 - refs [0, 1] map to service=api, pod=a
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service", "pod"},
			Values: []string{"api", "a"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 100, AnnotationRefs: []int64{1}},
			},
		}},
	})

	// Report 2 - refs [0, 1] map to pod=b, service=api (different order!)
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"pod", "service"},
			Values: []string{"b", "api"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{1}, // service=api
			Points: []*queryv1.Point{
				{Timestamp: 2000, Value: 200, AnnotationRefs: []int64{0}}, // pod=b
			},
		}},
	})

	// Both reports have same series (service=api), so should merge
	it := m.Iterator()
	var points []CompactValue
	for it.Next() {
		points = append(points, it.At())
	}

	require.Len(t, points, 2)
	// Same series key (both service=api)
	assert.Equal(t, points[0].SeriesKey, points[1].SeriesKey)

	// Verify attribute table has all unique entries
	table := m.BuildAttributeTable()
	assert.Len(t, table.Keys, 3) // service, pod(a), pod(b)
}

func TestMerger_WithExemplars(t *testing.T) {
	m := NewMerger()
	m.MergeReport(&queryv1.TimeSeriesCompactReport{
		AttributeTable: &queryv1.AttributeTable{
			Keys:   []string{"service", "pod"},
			Values: []string{"api", "a"},
		},
		TimeSeries: []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{{
				Timestamp: 1000,
				Value:     100,
				Exemplars: []*queryv1.Exemplar{{
					ProfileId:     "prof-1",
					Value:         100,
					AttributeRefs: []int64{1},
				}},
			}},
		}},
	})

	it := m.Iterator()
	require.True(t, it.Next())
	v := it.At()
	require.Len(t, v.Exemplars, 1)
	assert.Equal(t, "prof-1", v.Exemplars[0].ProfileId)
}
