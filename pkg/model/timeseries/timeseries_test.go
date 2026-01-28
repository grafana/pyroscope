package timeseries

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func TestBuilder_NoExemplarsForEmptyProfileID(t *testing.T) {
	builder := NewBuilder()
	labels := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "env", Value: "prod"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "")
	builder.Add(1, labels, 1000, 200.0, schemav1.Annotations{}, "")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 2)

	for _, point := range series[0].Points {
		assert.Empty(t, point.Exemplars, "Empty profileID should not create exemplars")
	}
}

func TestBuilder_Build_NoExemplars(t *testing.T) {
	builder := NewBuilder()
	labels := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

	series := builder.Build()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.Empty(t, series[0].Points[0].Exemplars, "Build() should not attach exemplars")
}

func TestBuilder_BuildWithExemplars_AttachesExemplars(t *testing.T) {
	builder := NewBuilder()
	labels := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-123"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	require.Len(t, series[0].Points[0].Exemplars, 1)

	exemplar := series[0].Points[0].Exemplars[0]
	assert.Equal(t, "profile-1", exemplar.ProfileId)
	assert.Equal(t, int64(100), exemplar.Value)
	assert.Equal(t, int64(1000), exemplar.Timestamp)

	assert.Len(t, exemplar.Labels, 2)
	assert.Equal(t, "api", findLabelValue(exemplar.Labels, "service_name"))
	assert.Equal(t, "pod-123", findLabelValue(exemplar.Labels, "pod"))
}

func TestBuilder_MultipleExemplarsAtSameTimestamp(t *testing.T) {
	builder := NewBuilder()
	labels := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")
	builder.Add(1, labels, 1000, 200.0, schemav1.Annotations{}, "profile-2")
	builder.Add(1, labels, 1000, 300.0, schemav1.Annotations{}, "profile-3")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 3)

	// All 3 points at timestamp 1000 should have all 3 exemplars
	for _, point := range series[0].Points {
		require.Len(t, point.Exemplars, 3)
		profileIDs := make(map[string]bool)
		for _, ex := range point.Exemplars {
			profileIDs[ex.ProfileId] = true
		}
		assert.True(t, profileIDs["profile-1"])
		assert.True(t, profileIDs["profile-2"])
		assert.True(t, profileIDs["profile-3"])
	}
}

func TestBuilder_GroupBy(t *testing.T) {
	builder := NewBuilder("service_name")
	labels1 := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-1"},
	}
	labels2 := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-2"},
	}

	builder.Add(1, labels1, 1000, 100.0, schemav1.Annotations{}, "profile-1")
	builder.Add(2, labels2, 1000, 200.0, schemav1.Annotations{}, "profile-2")

	series := builder.BuildWithExemplars()

	// Should be grouped into 1 series by service_name
	require.Len(t, series, 1)
	assert.Len(t, series[0].Labels, 1)
	assert.Equal(t, "service_name", series[0].Labels[0].Name)
	assert.Equal(t, "api", series[0].Labels[0].Value)

	require.Len(t, series[0].Points, 2)

	// Both exemplars should be at timestamp 1000, grouped together
	point := series[0].Points[0]
	require.Len(t, point.Exemplars, 2)

	// Exemplars should have only non-grouped labels (pod), not service_name
	for _, ex := range point.Exemplars {
		assert.Len(t, ex.Labels, 1)
		assert.NotEmpty(t, findLabelValue(ex.Labels, "pod"))
		assert.Empty(t, findLabelValue(ex.Labels, "service_name"))
	}
}

func TestBuilder_ExemplarDeduplication(t *testing.T) {
	builder := NewBuilder()
	labels := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-1"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-dup")
	builder.Add(1, labels, 1000, 200.0, schemav1.Annotations{}, "profile-dup")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 2)

	// Should deduplicate to 1 exemplar per point
	for _, point := range series[0].Points {
		require.Len(t, point.Exemplars, 1)
		assert.Equal(t, "profile-dup", point.Exemplars[0].ProfileId)
	}
}

func TestExemplarBuilder_SameProfileIDDifferentValues(t *testing.T) {
	builder := newExemplarBuilder()

	labels1 := phlaremodel.Labels{
		{Name: "pod", Value: "pod-1"},
	}
	labels2 := phlaremodel.Labels{
		{Name: "pod", Value: "pod-1"},
		{Name: "span_name", Value: "POST"},
	}

	builder.Add(1, labels1, 1000, "profile-123", 12830000000)
	builder.Add(2, labels2, 1000, "profile-123", 110000000)

	exemplars := builder.Build()
	require.Len(t, exemplars, 1)

	exemplar := exemplars[0]
	assert.Equal(t, "profile-123", exemplar.ProfileId)
	assert.Equal(t, int64(1000), exemplar.Timestamp)
	assert.Equal(t, int64(12940000000), exemplar.Value)

	// Labels should be intersected
	assert.Len(t, exemplar.Labels, 1)
	assert.Equal(t, "pod", exemplar.Labels[0].Name)
	assert.Equal(t, "pod-1", exemplar.Labels[0].Value)
}

func TestExemplarBuilder_DifferentProfileIDsNotSummed(t *testing.T) {
	builder := newExemplarBuilder()

	labels1 := phlaremodel.Labels{
		{Name: "pod", Value: "pod-1"},
		{Name: "span_name", Value: "POST"},
	}
	labels2 := phlaremodel.Labels{
		{Name: "pod", Value: "pod-2"},
		{Name: "span_name", Value: "POST"},
	}

	builder.Add(1, labels1, 1000, "profile-abc", 110000000)
	builder.Add(2, labels2, 1000, "profile-def", 150000000)

	exemplars := builder.Build()
	require.Len(t, exemplars, 2)

	// Sort by profile ID to ensure consistent ordering
	sort.Slice(exemplars, func(i, j int) bool {
		return exemplars[i].ProfileId < exemplars[j].ProfileId
	})

	// First exemplar
	assert.Equal(t, "profile-abc", exemplars[0].ProfileId)
	assert.Equal(t, int64(110000000), exemplars[0].Value)

	// Second exemplar
	assert.Equal(t, "profile-def", exemplars[1].ProfileId)
	assert.Equal(t, int64(150000000), exemplars[1].Value)
}

func TestBuilder_MultipleSeries(t *testing.T) {
	builder := NewBuilder("env")
	labels1 := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "env", Value: "prod"},
	}
	labels2 := phlaremodel.Labels{
		{Name: "service_name", Value: "api"},
		{Name: "env", Value: "staging"},
	}

	builder.Add(1, labels1, 1000, 100.0, schemav1.Annotations{}, "prod-profile")
	builder.Add(2, labels2, 1000, 200.0, schemav1.Annotations{}, "staging-profile")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 2)

	seriesByEnv := make(map[string]*typesv1.Series)
	for _, s := range series {
		for _, lp := range s.Labels {
			if lp.Name == "env" {
				seriesByEnv[lp.Value] = s
				break
			}
		}
	}

	prodSeries := seriesByEnv["prod"]
	require.NotNil(t, prodSeries)
	require.Len(t, prodSeries.Points, 1)
	require.Len(t, prodSeries.Points[0].Exemplars, 1)
	assert.Equal(t, "prod-profile", prodSeries.Points[0].Exemplars[0].ProfileId)

	stagingSeries := seriesByEnv["staging"]
	require.NotNil(t, stagingSeries)
	require.Len(t, stagingSeries.Points, 1)
	require.Len(t, stagingSeries.Points[0].Exemplars, 1)
	assert.Equal(t, "staging-profile", stagingSeries.Points[0].Exemplars[0].ProfileId)
}

func findLabelValue(labels []*typesv1.LabelPair, name string) string {
	for _, lp := range labels {
		if lp.Name == name {
			return lp.Value
		}
	}
	return ""
}
