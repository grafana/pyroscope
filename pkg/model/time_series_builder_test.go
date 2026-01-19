package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// TestTimeSeriesBuilder_BuildMethods tests the three build methods
func TestTimeSeriesBuilder_BuildMethods(t *testing.T) {
	labels := Labels{
		{Name: "service", Value: "api"},
		{Name: "pod", Value: "pod-1"},
	}

	t.Run("Build returns series without exemplars", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		series := builder.Build()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 1)
		assert.Empty(t, series[0].Points[0].Exemplars)
	})

	t.Run("BuildWithExemplars attaches exemplars with direct labels", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		series := builder.BuildWithExemplars()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 1)
		require.Len(t, series[0].Points[0].Exemplars, 1)

		ex := series[0].Points[0].Exemplars[0]
		assert.Equal(t, "profile-1", ex.ProfileId)
		assert.Equal(t, uint64(100), ex.Value)
		assert.Len(t, ex.Labels, 2)
	})

	t.Run("BuildWithAttributeTable uses attribute_refs for string interning", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		series := builder.BuildWithAttributeTable()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 1)
		require.Len(t, series[0].Points[0].Exemplars, 1)

		ex := series[0].Points[0].Exemplars[0]
		assert.NotNil(t, ex.AttributeRefs)
		assert.Len(t, ex.AttributeRefs, 2)
	})

	t.Run("Empty profile ID does not create exemplars", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "")

		series := builder.BuildWithExemplars()
		require.Len(t, series, 1)
		assert.Empty(t, series[0].Points[0].Exemplars)
	})
}

// TestTimeSeriesBuilder_GroupBy tests grouping behavior
func TestTimeSeriesBuilder_GroupBy(t *testing.T) {
	t.Run("Single group_by label", func(t *testing.T) {
		builder := NewTimeSeriesBuilder("service")
		builder.Add(1, Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-1"}}, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		builder.Add(2, Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-2"}}, 1000, 200.0, schemav1.Annotations{}, "profile-2")

		series := builder.BuildWithExemplars()

		require.Len(t, series, 1, "Should group into 1 series")
		assert.Equal(t, "service", series[0].Labels[0].Name)
		require.Len(t, series[0].Points, 2)

		// Exemplars should only have non-grouped labels
		for _, point := range series[0].Points {
			for _, ex := range point.Exemplars {
				assert.Len(t, ex.Labels, 1, "Should only have 'pod' label")
				assert.Equal(t, "pod", ex.Labels[0].Name)
			}
		}
	})

	t.Run("Multiple group_by labels", func(t *testing.T) {
		builder := NewTimeSeriesBuilder("service", "env")
		builder.Add(1, Labels{{Name: "service", Value: "api"}, {Name: "env", Value: "prod"}, {Name: "pod", Value: "pod-1"}}, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		builder.Add(2, Labels{{Name: "service", Value: "api"}, {Name: "env", Value: "prod"}, {Name: "pod", Value: "pod-2"}}, 1000, 200.0, schemav1.Annotations{}, "profile-2")

		series := builder.BuildWithExemplars()

		require.Len(t, series, 1, "Should group into 1 series")
		require.Len(t, series[0].Labels, 2, "Should have 2 group_by labels")
		require.Len(t, series[0].Points, 2)

		// Exemplars should only have 'pod' label
		for _, point := range series[0].Points {
			for _, ex := range point.Exemplars {
				assert.Len(t, ex.Labels, 1)
				assert.Equal(t, "pod", ex.Labels[0].Name)
			}
		}
	})

	t.Run("No group_by groups by full label set", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		// Same labels = same series
		labels := Labels{{Name: "service", Value: "api"}, {Name: "env", Value: "prod"}}
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		builder.Add(1, labels, 2000, 200.0, schemav1.Annotations{}, "profile-2")

		series := builder.BuildWithExemplars()

		require.Len(t, series, 1, "Same label set should create 1 series")
		require.Len(t, series[0].Points, 2, "Should have 2 points")

		// Exemplars include all labels since no group_by
		for _, point := range series[0].Points {
			for _, ex := range point.Exemplars {
				assert.Len(t, ex.Labels, 2, "Should include all labels")
			}
		}
	})
}

func TestTimeSeriesBuilder_ExemplarAggregation(t *testing.T) {
	labels := Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-1"}}

	t.Run("Multiple profiles at same timestamp are all attached", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		builder.Add(2, labels, 1000, 200.0, schemav1.Annotations{}, "profile-2")
		builder.Add(3, labels, 1000, 300.0, schemav1.Annotations{}, "profile-3")

		series := builder.BuildWithExemplars()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 3, "Each Add creates a point")

		// All points at same timestamp should have all exemplars
		for _, point := range series[0].Points {
			assert.Len(t, point.Exemplars, 3)
		}
	})

	t.Run("Same profile ID at same timestamp is deduplicated", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-dup")
		builder.Add(1, labels, 1000, 200.0, schemav1.Annotations{}, "profile-dup")

		series := builder.BuildWithExemplars()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 2)

		// Should have 1 exemplar per point (deduplicated)
		for _, point := range series[0].Points {
			assert.Len(t, point.Exemplars, 1)
			assert.Equal(t, "profile-dup", point.Exemplars[0].ProfileId)
		}
	})

	t.Run("Different timestamps create separate points with separate exemplars", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		builder.Add(1, labels, 2000, 200.0, schemav1.Annotations{}, "profile-2")

		series := builder.BuildWithExemplars()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 2)

		// Each point should have its own exemplar
		assert.Equal(t, int64(1000), series[0].Points[0].Timestamp)
		assert.Len(t, series[0].Points[0].Exemplars, 1)
		assert.Equal(t, "profile-1", series[0].Points[0].Exemplars[0].ProfileId)

		assert.Equal(t, int64(2000), series[0].Points[1].Timestamp)
		assert.Len(t, series[0].Points[1].Exemplars, 1)
		assert.Equal(t, "profile-2", series[0].Points[1].Exemplars[0].ProfileId)
	})
}

func TestTimeSeriesBuilder_Annotations(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{{Name: "service", Value: "api"}}
	annotations := schemav1.Annotations{
		Keys:   []string{"request_id"},
		Values: []string{"abc-123"},
	}

	builder.Add(1, labels, 1000, 100.0, annotations, "profile-1")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)

	point := series[0].Points[0]
	require.Len(t, point.Annotations, 1)
	assert.Equal(t, "request_id", point.Annotations[0].Key)
	assert.Equal(t, "abc-123", point.Annotations[0].Value)
}

func TestTimeSeriesBuilder_AttributeTable(t *testing.T) {
	t.Run("String interning works correctly", func(t *testing.T) {
		builder := NewTimeSeriesBuilder("service")
		builder.Add(1, Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-1"}, {Name: "region", Value: "us-east"}}, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		builder.Add(2, Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-2"}, {Name: "region", Value: "us-east"}}, 1000, 200.0, schemav1.Annotations{}, "profile-2")

		_ = builder.BuildWithAttributeTable()
		table := builder.AttributeTable().Build(nil)

		// Verify string interning: region=us-east should appear only once
		regionCount := 0
		for i, key := range table.Keys {
			if key == "region" && table.Values[i] == "us-east" {
				regionCount++
			}
		}
		assert.Equal(t, 1, regionCount, "String interning should deduplicate region=us-east")
	})

	t.Run("Attribute refs are valid indices", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-1"}}, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		series := builder.BuildWithAttributeTable()
		table := builder.AttributeTable().Build(nil)

		ex := series[0].Points[0].Exemplars[0]
		for _, ref := range ex.AttributeRefs {
			assert.GreaterOrEqual(t, ref, int64(0))
			assert.Less(t, ref, int64(len(table.Keys)))
		}
	})

	t.Run("Equivalent to BuildWithExemplars", func(t *testing.T) {
		labels := Labels{{Name: "service", Value: "api"}, {Name: "pod", Value: "pod-1"}}

		builder1 := NewTimeSeriesBuilder("service")
		builder1.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		builder2 := NewTimeSeriesBuilder("service")
		builder2.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		seriesWithLabels := builder1.BuildWithExemplars()
		seriesWithRefs := builder2.BuildWithAttributeTable()
		table := builder2.AttributeTable().Build(nil)

		// Series and points should match
		assert.Equal(t, seriesWithLabels[0].Labels, seriesWithRefs[0].Labels)
		assert.Equal(t, seriesWithLabels[0].Points[0].Value, seriesWithRefs[0].Points[0].Value)
		assert.Equal(t, seriesWithLabels[0].Points[0].Timestamp, seriesWithRefs[0].Points[0].Timestamp)

		// Exemplars should be equivalent after resolution
		ex1 := seriesWithLabels[0].Points[0].Exemplars[0]
		ex2 := seriesWithRefs[0].Points[0].Exemplars[0]
		assert.Equal(t, ex1.ProfileId, ex2.ProfileId)
		assert.Equal(t, ex1.Value, ex2.Value)

		resolvedLabels := resolveAttributeRefs(t, ex2.AttributeRefs, table)
		assert.ElementsMatch(t, ex1.Labels, resolvedLabels)
	})

	t.Run("Empty when no exemplars", func(t *testing.T) {
		builder := NewTimeSeriesBuilder()
		builder.Add(1, Labels{{Name: "service", Value: "api"}}, 1000, 100.0, schemav1.Annotations{}, "")

		series := builder.BuildWithAttributeTable()
		table := builder.AttributeTable().Build(nil)

		assert.Empty(t, series[0].Points[0].Exemplars)
		assert.Empty(t, table.Keys)
		assert.Empty(t, table.Values)
	})
}

// Helper functions
func resolveAttributeRefs(t *testing.T, refs []int64, table *queryv1.AttributeTable) []*typesv1.LabelPair {
	t.Helper()
	labels := make([]*typesv1.LabelPair, len(refs))
	for i, ref := range refs {
		labels[i] = &typesv1.LabelPair{
			Name:  table.Keys[ref],
			Value: table.Values[ref],
		}
	}
	return labels
}
