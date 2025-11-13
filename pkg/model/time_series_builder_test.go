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

	series := builder.Build()
	require.Len(t, series, 1)
	require.Len(t, series[0].Points, 2)

	for _, point := range series[0].Points {
		assert.Empty(t, point.Exemplars, "V1 queries should not have exemplars")
	}
}

func TestTimeSeriesBuilder_KeepsHighestValueExemplar(t *testing.T) {
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
		require.Len(t, point.Exemplars, 1)
		assert.Equal(t, "profile-high", point.Exemplars[0].ProfileId)
		assert.Equal(t, uint64(500), point.Exemplars[0].Value)
	}
}

func TestTimeSeriesBuilder_MultipleExemplarsPerPoint(t *testing.T) {
	builder := NewTimeSeriesBuilder()
	builder.maxExemplarsPerPoint = 2
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
	require.Len(t, point.Exemplars, 2)

	profileIDs := make(map[string]uint64)
	for _, ex := range point.Exemplars {
		profileIDs[ex.ProfileId] = ex.Value
	}

	assert.Contains(t, profileIDs, "profile-2")
	assert.Equal(t, uint64(500), profileIDs["profile-2"])
	assert.Contains(t, profileIDs, "profile-3")
	assert.Equal(t, uint64(200), profileIDs["profile-3"])
	assert.NotContains(t, profileIDs, "profile-1")
}

func TestTimeSeriesBuilder_LabelFiltering(t *testing.T) {
	tests := []struct {
		name               string
		groupBy            []string
		inputLabels        Labels
		expectedInExemplar map[string]string
		expectedExcluded   []string
	}{
		{
			name:    "single grouped label excluded",
			groupBy: []string{"service_name"},
			inputLabels: Labels{
				{Name: "service_name", Value: "api"},
				{Name: "env", Value: "prod"},
				{Name: "pod", Value: "pod-123"},
			},
			expectedInExemplar: map[string]string{
				"env": "prod",
				"pod": "pod-123",
			},
			expectedExcluded: []string{"service_name"},
		},
		{
			name:    "multiple grouped labels excluded",
			groupBy: []string{"service_name", "env"},
			inputLabels: Labels{
				{Name: "service_name", Value: "api"},
				{Name: "env", Value: "prod"},
				{Name: "pod", Value: "pod-123"},
				{Name: "region", Value: "us-east"},
			},
			expectedInExemplar: map[string]string{
				"pod":    "pod-123",
				"region": "us-east",
			},
			expectedExcluded: []string{"service_name", "env"},
		},
		{
			name:    "no grouping includes all labels",
			groupBy: []string{},
			inputLabels: Labels{
				{Name: "service_name", Value: "api"},
				{Name: "env", Value: "prod"},
				{Name: "pod", Value: "pod-123"},
			},
			expectedInExemplar: map[string]string{
				"service_name": "api",
				"env":          "prod",
				"pod":          "pod-123",
			},
			expectedExcluded: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewTimeSeriesBuilder(tt.groupBy...)

			builder.Add(
				model.Fingerprint(1),
				tt.inputLabels,
				1000,
				100.0,
				schemav1.Annotations{},
				"profile-1",
			)

			series := builder.Build()
			require.Len(t, series, 1)
			require.Len(t, series[0].Points, 1)
			require.Len(t, series[0].Points[0].Exemplars, 1)

			exemplar := series[0].Points[0].Exemplars[0]
			exemplarLabels := make(map[string]string)
			for _, lp := range exemplar.Labels {
				exemplarLabels[lp.Name] = lp.Value
			}

			for name, expectedValue := range tt.expectedInExemplar {
				assert.Equal(t, expectedValue, exemplarLabels[name])
			}

			for _, name := range tt.expectedExcluded {
				assert.NotContains(t, exemplarLabels, name)
			}

			assert.Len(t, exemplarLabels, len(tt.expectedInExemplar))
		})
	}
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
