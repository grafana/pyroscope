package querybackend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestTimeSeriesAggregator_FilterLabels(t *testing.T) {
	tests := []struct {
		name           string
		groupBy        []string
		inputLabels    []string
		expectSeries   []string
		expectExemplar []string
	}{
		{
			name:           "single groupBy label",
			groupBy:        []string{"service_name"},
			inputLabels:    []string{"service_name", "env", "pod"},
			expectSeries:   []string{"service_name"},
			expectExemplar: []string{"env", "pod"},
		},
		{
			name:           "multiple groupBy labels",
			groupBy:        []string{"service_name", "env"},
			inputLabels:    []string{"service_name", "env", "pod", "region"},
			expectSeries:   []string{"service_name", "env"},
			expectExemplar: []string{"pod", "region"},
		},
		{
			name:           "all labels grouped",
			groupBy:        []string{"service_name", "env"},
			inputLabels:    []string{"service_name", "env"},
			expectSeries:   []string{"service_name", "env"},
			expectExemplar: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputSeries := []*typesv1.Series{
				{
					Labels: makeLabels(t, tt.inputLabels),
					Points: []*typesv1.Point{
						{
							Timestamp: 1000,
							Value:     100,
							Exemplars: []*typesv1.Exemplar{
								{
									ProfileId: "test-profile",
									Value:     100,
									Labels:    makeLabels(t, tt.inputLabels),
								},
							},
						},
					},
				},
			}

			agg := &timeSeriesAggregator{
				query: &queryv1.TimeSeriesQuery{GroupBy: tt.groupBy},
			}

			result := agg.filterLabels(inputSeries, tt.groupBy)
			require.Len(t, result, 1)

			series := result[0]
			assert.ElementsMatch(t, tt.expectSeries, extractLabelNames(t, series.Labels))
			require.Len(t, series.Points, 1)
			require.Len(t, series.Points[0].Exemplars, 1)
			assert.ElementsMatch(t, tt.expectExemplar, extractLabelNames(t, series.Points[0].Exemplars[0].Labels))
		})
	}
}

func makeLabels(t *testing.T, names []string) []*typesv1.LabelPair {
	t.Helper()
	labels := make([]*typesv1.LabelPair, len(names))
	for i, name := range names {
		labels[i] = &typesv1.LabelPair{Name: name, Value: "value-" + name}
	}
	return labels
}

func extractLabelNames(t *testing.T, labels []*typesv1.LabelPair) []string {
	t.Helper()
	names := make([]string, len(labels))
	for i, lp := range labels {
		names[i] = lp.Name
	}
	return names
}
