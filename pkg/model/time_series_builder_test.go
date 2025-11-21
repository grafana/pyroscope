package model

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func TestTimeSeriesBuilder_NoExemplarsForEmptyProfileID(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
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

func TestTimeSeriesBuilder_Build_NoExemplars(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
		{Name: "service_name", Value: "api"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

	series := builder.Build()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 1)
	assert.Empty(t, series[0].Points[0].Exemplars, "Build() should not attach exemplars")
}

func TestTimeSeriesBuilder_BuildWithExemplars_AttachesExemplars(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
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
	assert.Equal(t, uint64(100), exemplar.Value)
	assert.Equal(t, int64(1000), exemplar.Timestamp)

	assert.Len(t, exemplar.Labels, 2)
	assert.Equal(t, "api", findLabelValue(exemplar.Labels, "service_name"))
	assert.Equal(t, "pod-123", findLabelValue(exemplar.Labels, "pod"))
}

func TestTimeSeriesBuilder_MultipleExemplarsAtSameTimestamp(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
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

func TestTimeSeriesBuilder_GroupBy(t *testing.T) {
	builder := NewTimeSeriesBuilder("service_name")
	labels1 := Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-1"},
	}
	labels2 := Labels{
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

func TestTimeSeriesBuilder_ExemplarDeduplication(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
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

func TestTimeSeriesBuilder_ExemplarLabelIntersection(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels1 := Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-1"},
		{Name: "region", Value: "us-east"},
	}
	labels2 := Labels{
		{Name: "service_name", Value: "api"},
		{Name: "pod", Value: "pod-2"}, // Different pod
		{Name: "region", Value: "us-east"},
	}

	fp1 := model.Fingerprint(labels1.Hash())
	fp2 := model.Fingerprint(labels2.Hash())

	builder.Add(fp1, labels1, 1000, 100.0, schemav1.Annotations{}, "profile-dup")
	builder.Add(fp2, labels2, 1000, 200.0, schemav1.Annotations{}, "profile-dup")

	series := builder.BuildWithExemplars()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 2)

	// Find the exemplar (should be on one of the points)
	var exemplar *typesv1.Exemplar
	for _, point := range series[0].Points {
		if len(point.Exemplars) > 0 {
			exemplar = point.Exemplars[0]
			break
		}
	}
	require.NotNil(t, exemplar, "Should have at least one exemplar")

	// Should only have labels that match (service_name and region, not pod)
	assert.Equal(t, "profile-dup", exemplar.ProfileId)
	assert.Equal(t, "api", findLabelValue(exemplar.Labels, "service_name"))
	assert.Equal(t, "us-east", findLabelValue(exemplar.Labels, "region"))
	assert.Empty(t, findLabelValue(exemplar.Labels, "pod"), "Dynamic label should be removed")
}

func TestTimeSeriesBuilder_MultipleSeries(t *testing.T) {
	builder := NewTimeSeriesBuilder("env")
	labels1 := Labels{
		{Name: "service_name", Value: "api"},
		{Name: "env", Value: "prod"},
	}
	labels2 := Labels{
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
