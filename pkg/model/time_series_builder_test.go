package model

import (
	"fmt"
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

	series := builder.Build()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 2)

	for _, point := range series[0].Points {
		assert.Empty(t, point.Exemplars, "V1 queries should not have exemplars")
	}
}

func TestTimeSeriesBuilder_KeepsAllExemplars(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
		{Name: "service_name", Value: "api"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-low")
	builder.Add(1, labels, 1000, 500.0, schemav1.Annotations{}, "profile-high")
	builder.Add(1, labels, 1000, 200.0, schemav1.Annotations{}, "profile-mid")

	series := builder.Build()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 3)

	for _, point := range series[0].Points {
		require.Len(t, point.Exemplars, 3)

		profileIDs := make(map[string]uint64)
		for _, ex := range point.Exemplars {
			profileIDs[ex.ProfileId] = ex.Value
		}

		assert.Contains(t, profileIDs, "profile-low")
		assert.Equal(t, uint64(100), profileIDs["profile-low"])
		assert.Contains(t, profileIDs, "profile-high")
		assert.Equal(t, uint64(500), profileIDs["profile-high"])
		assert.Contains(t, profileIDs, "profile-mid")
		assert.Equal(t, uint64(200), profileIDs["profile-mid"])
	}
}

func TestTimeSeriesBuilder_KeepsAllExemplarsWithMultiplePoints(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	labels := Labels{
		{Name: "service_name", Value: "api"},
	}

	builder.Add(1, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")
	builder.Add(1, labels, 1000, 500.0, schemav1.Annotations{}, "profile-2")
	builder.Add(1, labels, 1000, 200.0, schemav1.Annotations{}, "profile-3")

	series := builder.Build()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 3)

	point := series[0].Points[0]
	require.Len(t, point.Exemplars, 3)

	profileIDs := make(map[string]uint64)
	for _, ex := range point.Exemplars {
		profileIDs[ex.ProfileId] = ex.Value
	}

	assert.Contains(t, profileIDs, "profile-1")
	assert.Equal(t, uint64(100), profileIDs["profile-1"])
	assert.Contains(t, profileIDs, "profile-2")
	assert.Equal(t, uint64(500), profileIDs["profile-2"])
	assert.Contains(t, profileIDs, "profile-3")
	assert.Equal(t, uint64(200), profileIDs["profile-3"])
}

func TestTimeSeriesBuilder_ExemplarLabelEnrichment(t *testing.T) {
	t.Run("BuildWithFullLabels attaches provided labels", func(t *testing.T) {
		builder := NewTimeSeriesBuilder("service_name")
		fp := model.Fingerprint(1)

		fullLabels := Labels{
			{Name: "service_name", Value: "api"},
			{Name: "env", Value: "prod"},
			{Name: "pod", Value: "pod-123"},
		}

		builder.Add(fp, fullLabels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		series := builder.BuildWithFullLabels(map[model.Fingerprint]Labels{
			fp: fullLabels,
		})

		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 1)
		require.Len(t, series[0].Points[0].Exemplars, 1)

		exemplar := series[0].Points[0].Exemplars[0]
		assert.Len(t, exemplar.Labels, 3)
		assert.Equal(t, "api", findLabelValue(exemplar.Labels, "service_name"))
		assert.Equal(t, "prod", findLabelValue(exemplar.Labels, "env"))
		assert.Equal(t, "pod-123", findLabelValue(exemplar.Labels, "pod"))
	})

	t.Run("Build without labels map leaves exemplars with nil labels", func(t *testing.T) {
		builder := NewTimeSeriesBuilder("service_name")
		fp := model.Fingerprint(1)

		labels := Labels{
			{Name: "service_name", Value: "api"},
			{Name: "env", Value: "prod"},
		}

		builder.Add(fp, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")

		series := builder.Build()
		require.Len(t, series, 1)
		require.Len(t, series[0].Points, 1)
		require.Len(t, series[0].Points[0].Exemplars, 1)

		exemplar := series[0].Points[0].Exemplars[0]
		assert.Nil(t, exemplar.Labels)
	})

	t.Run("Missing fingerprint in map results in nil labels", func(t *testing.T) {
		builder := NewTimeSeriesBuilder("service_name")
		fp := model.Fingerprint(1)

		labels := Labels{
			{Name: "service_name", Value: "api"},
		}

		builder.Add(fp, labels, 1000, 100.0, schemav1.Annotations{}, "profile-1")
		series := builder.BuildWithFullLabels(map[model.Fingerprint]Labels{
			model.Fingerprint(999): labels,
		})
		require.Len(t, series, 1)

		exemplar := series[0].Points[0].Exemplars[0]
		assert.Nil(t, exemplar.Labels)
	})
}

func findLabelValue(labels []*typesv1.LabelPair, name string) string {
	for _, lp := range labels {
		if lp.Name == name {
			return lp.Value
		}
	}
	return ""
}

func TestTimeSeriesBuilder_RespectsMaxExemplarLimit(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	builder.maxExemplarCandidates = 2
	labels := Labels{
		{Name: "service_name", Value: "api"},
	}

	for i := 0; i < 3; i++ {
		profileID := fmt.Sprintf("profile-%d", i)
		builder.Add(1, labels, 1000, float64(i), schemav1.Annotations{}, profileID)
	}

	series := builder.Build()
	require.Len(t, series, 1)
	point := series[0].Points[0]

	assert.Len(t, point.Exemplars, 2)
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

	series := builder.Build()
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
