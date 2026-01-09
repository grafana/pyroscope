package heatmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	prommodel "github.com/prometheus/common/model"
)

func TestHeatmapBuilder(t *testing.T) {
	tests := []struct {
		name     string
		groupBy  []string
		input    []testPoint
		expected []testSeries
	}{
		{
			name:    "adds single point",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
					timestamp: 1001,
					profileID: "profile1",
					spanID:    0xa,
					value:     1,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
							timestamp: 1001,
							profileID: "profile1",
							spanID:    0xa,
							value:     1,
						},
					},
				},
			},
		},
		{
			name:    "adds two points, same series",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
					timestamp: 1001,
					profileID: "profile1",
					spanID:    0xa,
					value:     1,
				},
				{
					labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
					timestamp: 1002,
					profileID: "profile2",
					spanID:    0xb,
					value:     2,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
							timestamp: 1001,
							profileID: "profile1",
							spanID:    0xa,
							value:     1,
						},
						{
							labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
							timestamp: 1002,
							profileID: "profile2",
							spanID:    0xb,
							value:     2,
						},
					},
				},
			},
		},
		{
			name:    "adds two points, two series",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
					timestamp: 1001,
					profileID: "profile-1",
					spanID:    0xa,
					value:     1,
				},
				{
					labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod2"}},
					timestamp: 1002,
					profileID: "profile-2",
					spanID:    0xb,
					value:     2,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod1"}},
							timestamp: 1001,
							profileID: "profile-1",
							spanID:    0xa,
							value:     1,
						},
					},
				},
				{
					labels: model.Labels{{Name: "pod", Value: "pod2"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "namespace", Value: "namespace1"}, {Name: "pod", Value: "pod2"}},
							timestamp: 1002,
							profileID: "profile-2",
							spanID:    0xb,
							value:     2,
						},
					},
				},
			},
		},
		{
			name:    "adds multiple points with different timestamps",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 2000,
					profileID: "profile1",
					spanID:    0xa,
					value:     200,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1500,
					profileID: "profile1",
					spanID:    0xa,
					value:     150,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1500,
							profileID: "profile1",
							spanID:    0xa,
							value:     150,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 2000,
							profileID: "profile1",
							spanID:    0xa,
							value:     200,
						},
					},
				},
			},
		},
		{
			name:    "merges points with same timestamp, profileID, and spanID",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     50,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     150,
						},
					},
				},
			},
		},
		{
			name:    "handles different series with different fingerprints",
			groupBy: []string{"common"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "common", Value: "value"}, {Name: "label1", Value: "value1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "common", Value: "value"}, {Name: "label1", Value: "value2"}, {Name: "random", Value: "stuff"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     200,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "common", Value: "value"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "common", Value: "value"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     300,
						},
					},
				},
			},
		},
		{
			name:    "handles different profileIDs",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile2",
					spanID:    0xa,
					value:     200,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile2",
							spanID:    0xa,
							value:     200,
						},
					},
				},
			},
		},
		{
			name:    "handles different spanIDs",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xb,
					value:     200,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xb,
							value:     200,
						},
					},
				},
			},
		},
		{
			name:    "maintains sorted order with complex insertions",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 3000,
					profileID: "profile1",
					spanID:    0xa,
					value:     300,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 5000,
					profileID: "profile1",
					spanID:    0xa,
					value:     500,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 2000,
					profileID: "profile1",
					spanID:    0xa,
					value:     200,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 4000,
					profileID: "profile1",
					spanID:    0xa,
					value:     400,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 2000,
							profileID: "profile1",
							spanID:    0xa,
							value:     200,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 3000,
							profileID: "profile1",
							spanID:    0xa,
							value:     300,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 4000,
							profileID: "profile1",
							spanID:    0xa,
							value:     400,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 5000,
							profileID: "profile1",
							spanID:    0xa,
							value:     500,
						},
					},
				},
			},
		},
		{
			name:    "reuses series when same fingerprint",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 2000,
					profileID: "profile1",
					spanID:    0xa,
					value:     200,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 3000,
					profileID: "profile1",
					spanID:    0xa,
					value:     300,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 2000,
							profileID: "profile1",
							spanID:    0xa,
							value:     200,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 3000,
							profileID: "profile1",
							spanID:    0xa,
							value:     300,
						},
					},
				},
			},
		},
		{
			name:    "handles zero values",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     0,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     0,
						},
					},
				},
			},
		},
		{
			name:    "handles empty labels",
			groupBy: []string{},
			input: []testPoint{
				{
					labels:    model.Labels{},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{},
					points: []testPoint{
						{
							labels:    model.Labels{},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
					},
				},
			},
		},
		{
			name:    "handles multiple series with interleaved timestamps",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "profile1",
					spanID:    0xa,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod2"}},
					timestamp: 1500,
					profileID: "profile1",
					spanID:    0xa,
					value:     150,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 2000,
					profileID: "profile1",
					spanID:    0xa,
					value:     200,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod2"}},
					timestamp: 2500,
					profileID: "profile1",
					spanID:    0xa,
					value:     250,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1000,
							profileID: "profile1",
							spanID:    0xa,
							value:     100,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 2000,
							profileID: "profile1",
							spanID:    0xa,
							value:     200,
						},
					},
				},
				{
					labels: model.Labels{{Name: "pod", Value: "pod2"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod2"}},
							timestamp: 1500,
							profileID: "profile1",
							spanID:    0xa,
							value:     150,
						},
						{
							labels:    model.Labels{{Name: "pod", Value: "pod2"}},
							timestamp: 2500,
							profileID: "profile1",
							spanID:    0xa,
							value:     250,
						},
					},
				},
			},
		},
		{
			name:    "skips points with empty profileID and zero spanID",
			groupBy: []string{"pod"},
			input: []testPoint{
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1000,
					profileID: "",
					spanID:    0,
					value:     100,
				},
				{
					labels:    model.Labels{{Name: "pod", Value: "pod1"}},
					timestamp: 1001,
					profileID: "profile1",
					spanID:    0xa,
					value:     200,
				},
			},
			expected: []testSeries{
				{
					labels: model.Labels{{Name: "pod", Value: "pod1"}},
					points: []testPoint{
						{
							labels:    model.Labels{{Name: "pod", Value: "pod1"}},
							timestamp: 1001,
							profileID: "profile1",
							spanID:    0xa,
							value:     200,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder(tt.groupBy)

			// Add all input points
			for _, p := range tt.input {
				fp := prommodel.Fingerprint(p.labels.Hash())
				builder.Add(fp, p.labels, p.timestamp, p.profileID, p.spanID, p.value)
			}

			result := builder.Build(nil)

			// Convert result back to testSeries format
			actual := resultToTestSeries(result)

			// Compare expected vs actual
			require.Len(t, actual, len(tt.expected), "number of series mismatch")

			for i, expectedSeries := range tt.expected {
				actualSeries := actual[i]

				// Compare series labels
				assert.Equal(t, expectedSeries.labels, actualSeries.labels,
					"series %d labels mismatch", i)

				// Compare points
				require.Len(t, actualSeries.points, len(expectedSeries.points),
					"series %d point count mismatch", i)

				for j, expectedPoint := range expectedSeries.points {
					actualPoint := actualSeries.points[j]

					assert.Equal(t, expectedPoint.labels, actualPoint.labels,
						"series %d point %d labels mismatch", i, j)
					assert.Equal(t, expectedPoint.timestamp, actualPoint.timestamp,
						"series %d point %d timestamp mismatch", i, j)
					assert.Equal(t, expectedPoint.profileID, actualPoint.profileID,
						"series %d point %d profileID mismatch", i, j)
					assert.Equal(t, expectedPoint.spanID, actualPoint.spanID,
						"series %d point %d spanID mismatch", i, j)
					assert.Equal(t, expectedPoint.value, actualPoint.value,
						"series %d point %d value mismatch", i, j)
				}
			}
		})
	}
}

// testPoint represents a test data point
type testPoint struct {
	labels    model.Labels
	timestamp int64
	profileID string
	spanID    uint64
	value     uint64
}

// testSeries represents a series with its points for testing
type testSeries struct {
	labels model.Labels
	points []testPoint
}

// resultToTestSeries converts a HeatmapReport back to testSeries format for comparison
func resultToTestSeries(report *queryv1.HeatmapReport) []testSeries {
	result := make([]testSeries, len(report.HeatmapSeries))

	for i, series := range report.HeatmapSeries {
		// Convert series labels
		seriesLabels := make(model.Labels, len(series.AttributeRefs))
		for j, ref := range series.AttributeRefs {
			seriesLabels[j] = &typesv1.LabelPair{
				Name:  report.AttributeTable.Keys[ref],
				Value: report.AttributeTable.Values[ref],
			}
		}

		// Convert points
		points := make([]testPoint, len(series.Points))
		for j, p := range series.Points {
			// Convert point labels
			pointLabels := make(model.Labels, len(p.AttributeRefs))
			for k, ref := range p.AttributeRefs {
				pointLabels[k] = &typesv1.LabelPair{
					Name:  report.AttributeTable.Keys[ref],
					Value: report.AttributeTable.Values[ref],
				}
			}

			points[j] = testPoint{
				labels:    pointLabels,
				timestamp: p.Timestamp,
				profileID: report.AttributeTable.Values[p.ProfileId],
				spanID:    p.SpanId,
				value:     p.Value,
			}
		}

		result[i] = testSeries{
			labels: seriesLabels,
			points: points,
		}
	}

	return result
}
