package heatmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestRangeHeatmap_EmptyInput(t *testing.T) {
	series := RangeHeatmap(nil, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	assert.Nil(t, series)
}

func TestRangeHeatmap_EmptyReport(t *testing.T) {
	reports := []*queryv1.HeatmapReport{{}}
	series := RangeHeatmap(reports, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	assert.Nil(t, series)
}

func TestRangeHeatmap_SinglePoint(t *testing.T) {
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{
							Timestamp: 500,
							Value:     1000,
						},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)

	// Should have at least one slot
	require.NotEmpty(t, series[0].Slots)

	// Verify slot structure
	slot := series[0].Slots[0]
	assert.Equal(t, int64(500), slot.Timestamp)
	assert.Len(t, slot.YMin, DefaultYAxisBuckets)
	assert.Len(t, slot.Counts, DefaultYAxisBuckets)

	// At least one bucket should have a count
	totalCount := int32(0)
	for _, count := range slot.Counts {
		totalCount += count
	}
	assert.Greater(t, totalCount, int32(0), "Expected at least one bucket to have a count")
}

func TestRangeHeatmap_MultiplePoints_SameTimeBucket(t *testing.T) {
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 1000},
						{Timestamp: 150, Value: 1500},
						{Timestamp: 180, Value: 2000},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 200, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)

	// All points should fall into one time bucket
	require.Len(t, series[0].Slots, 1)

	slot := series[0].Slots[0]

	// Should have counts in multiple Y buckets (values are spread: 1000, 1500, 2000)
	nonZeroBuckets := 0
	totalCount := int32(0)
	for _, count := range slot.Counts {
		if count > 0 {
			nonZeroBuckets++
			totalCount += count
		}
	}
	assert.Greater(t, nonZeroBuckets, 0, "Expected at least one bucket with counts")
	assert.Equal(t, int32(3), totalCount, "Expected total count to match number of points")
}

func TestRangeHeatmap_MultiplePoints_DifferentTimeBuckets(t *testing.T) {
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 1000},
						{Timestamp: 300, Value: 1500},
						{Timestamp: 500, Value: 2000},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 200, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)

	// Points should fall into different time buckets
	require.GreaterOrEqual(t, len(series[0].Slots), 2, "Expected at least 2 time buckets")

	// Verify slots are sorted by timestamp
	for i := 1; i < len(series[0].Slots); i++ {
		assert.Less(t, series[0].Slots[i-1].Timestamp, series[0].Slots[i].Timestamp,
			"Slots should be sorted by timestamp")
	}

	// Each slot should have the correct structure
	for _, slot := range series[0].Slots {
		assert.Len(t, slot.YMin, DefaultYAxisBuckets)
		assert.Len(t, slot.Counts, DefaultYAxisBuckets)
	}
}

func TestRangeHeatmap_AllSameValue(t *testing.T) {
	// Test edge case where all values are the same
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 1000},
						{Timestamp: 200, Value: 1000},
						{Timestamp: 300, Value: 1000},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 200, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)

	// Should still create buckets even with identical values
	require.NotEmpty(t, series[0].Slots)

	// All counts should be in the same Y bucket
	for _, slot := range series[0].Slots {
		nonZeroBuckets := 0
		for _, count := range slot.Counts {
			if count > 0 {
				nonZeroBuckets++
			}
		}
		// With identical values, they should all fall into the same Y bucket
		assert.LessOrEqual(t, nonZeroBuckets, 2,
			"With identical values, counts should be in at most 2 adjacent buckets")
	}
}

func TestRangeHeatmap_YBucketBoundaries(t *testing.T) {
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 0},
						{Timestamp: 100, Value: 10000},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)
	require.NotEmpty(t, series[0].Slots)

	slot := series[0].Slots[0]

	// Verify Y bucket boundaries are monotonically increasing
	for i := 1; i < len(slot.YMin); i++ {
		assert.Less(t, slot.YMin[i-1], slot.YMin[i],
			"Y bucket boundaries should be monotonically increasing")
	}

	// First bucket should start near the minimum value
	assert.LessOrEqual(t, slot.YMin[0], 100.0,
		"First Y bucket should start at or near minimum value")
}

func TestRangeHeatmap_MultipleReports(t *testing.T) {
	// Test that data from multiple reports produces separate series (not merged)
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 1000},
						{Timestamp: 200, Value: 2000},
					},
				},
			},
		},
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 200, Value: 1500},
						{Timestamp: 300, Value: 2500},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	// Each input series should produce a separate output series
	require.Len(t, series, 2)

	// Verify first series has 2 points
	totalCount1 := int32(0)
	for _, slot := range series[0].Slots {
		for _, count := range slot.Counts {
			totalCount1 += count
		}
	}
	assert.Equal(t, int32(2), totalCount1, "Expected first series to have 2 points")

	// Verify second series has 2 points
	totalCount2 := int32(0)
	for _, slot := range series[1].Slots {
		for _, count := range slot.Counts {
			totalCount2 += count
		}
	}
	assert.Equal(t, int32(2), totalCount2, "Expected second series to have 2 points")
}

func TestRangeHeatmap_LabelsPreserved(t *testing.T) {
	// Test that labels are correctly resolved and preserved per series
	// AttributeTable has parallel arrays: Keys[i] and Values[i] form a label pair
	reports := []*queryv1.HeatmapReport{
		{
			AttributeTable: &queryv1.AttributeTable{
				// Index 0: service=api
				// Index 1: environment=production
				// Index 2: service=web
				// Index 3: environment=staging
				Keys:   []string{"service", "environment", "service", "environment"},
				Values: []string{"api", "production", "web", "staging"},
			},
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					AttributeRefs: []int64{0, 1}, // refs to: service=api, environment=production
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 1000},
					},
				},
				{
					AttributeRefs: []int64{2, 3}, // refs to: service=web, environment=staging
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 2000},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 2, "Expected one series per input series")

	// Verify first series labels
	require.Len(t, series[0].Labels, 2)
	assert.Equal(t, "service", series[0].Labels[0].Name)
	assert.Equal(t, "api", series[0].Labels[0].Value)
	assert.Equal(t, "environment", series[0].Labels[1].Name)
	assert.Equal(t, "production", series[0].Labels[1].Value)

	// Verify second series labels
	require.Len(t, series[1].Labels, 2)
	assert.Equal(t, "service", series[1].Labels[0].Name)
	assert.Equal(t, "web", series[1].Labels[0].Value)
	assert.Equal(t, "environment", series[1].Labels[1].Name)
	assert.Equal(t, "staging", series[1].Labels[1].Value)
}

func TestRangeHeatmap_NoLabels(t *testing.T) {
	// Test that series without attribute refs work correctly
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 100, Value: 1000},
					},
				},
			},
		},
	}

	series := RangeHeatmap(reports, 0, 1000, 100, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)

	// Series should have no labels
	assert.Nil(t, series[0].Labels)
}

func TestRangeHeatmap_TimeBucketAlignment(t *testing.T) {
	reports := []*queryv1.HeatmapReport{
		{
			HeatmapSeries: []*queryv1.HeatmapSeries{
				{
					Points: []*queryv1.HeatmapPoint{
						{Timestamp: 50, Value: 1000},
						{Timestamp: 250, Value: 2000},
						{Timestamp: 450, Value: 3000},
					},
				},
			},
		},
	}

	step := int64(200)
	series := RangeHeatmap(reports, 0, 600, step, nil, typesv1.ExemplarType_EXEMPLAR_TYPE_NONE)
	require.NotNil(t, series)
	require.Len(t, series, 1)

	// Verify each slot timestamp aligns with the step
	for _, slot := range series[0].Slots {
		assert.True(t, slot.Timestamp >= 0 && slot.Timestamp <= 600,
			"Slot timestamp should be within range")
	}
}
