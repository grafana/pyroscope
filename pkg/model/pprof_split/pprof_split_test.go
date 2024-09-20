package pprof_split

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/assert"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

type sampleSeries struct {
	labels  []*typesv1.LabelPair
	samples []*profilev1.Sample
}

type mockVisitor struct {
	labels            []*typesv1.LabelPair
	visited           []sampleSeries
	discardedBytes    int
	discardedProfiles int
}

func (m *mockVisitor) VisitProfile(labels []*typesv1.LabelPair) {
	m.labels = labels
}

func (m *mockVisitor) VisitSampleSeries(labels []*typesv1.LabelPair, samples []*profilev1.Sample) {
	m.visited = append(m.visited, sampleSeries{
		labels:  labels,
		samples: samples,
	})
}

func (m *mockVisitor) Discarded(profiles, bytes int) {
	m.discardedBytes += bytes
	m.discardedProfiles += profiles
}

func Test_VisitSampleSeries(t *testing.T) {
	defaultRelabelConfigs := validation.MockDefaultOverrides().IngestionRelabelingRules("")

	type testCase struct {
		description string
		rules       []*relabel.Config
		labels      []*typesv1.LabelPair
		profile     *profilev1.Profile

		expected       []sampleSeries
		expectNoSeries bool
		expectLabels   []*typesv1.LabelPair

		expectBytesDropped    int
		expectProfilesDropped int
	}

	testCases := []testCase{
		{
			description: "no series labels, no sample labels",
			profile: &profilev1.Profile{
				Sample: []*profilev1.Sample{{
					Value: []int64{1},
				}},
			},
			expectNoSeries: true,
			expectLabels:   nil,
		},
		{
			description: "has series labels, no sample labels",
			labels: []*typesv1.LabelPair{
				{Name: "foo", Value: "bar"},
			},
			profile: &profilev1.Profile{
				Sample: []*profilev1.Sample{{
					Value: []int64{1},
				}},
			},
			expectNoSeries: true,
			expectLabels: []*typesv1.LabelPair{
				{Name: "foo", Value: "bar"},
			},
		},
		{
			description: "no series labels, all samples have identical label set",
			profile: &profilev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Sample: []*profilev1.Sample{{
					Value: []int64{1},
					Label: []*profilev1.Label{
						{Key: 1, Str: 2},
					},
				}},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "foo", Value: "bar"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{1},
						Label: []*profilev1.Label{},
					}},
				},
			},
		},
		{
			description: "has series labels, all samples have identical label set",
			labels: []*typesv1.LabelPair{
				{Name: "baz", Value: "qux"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Sample: []*profilev1.Sample{{
					Value: []int64{1},
					Label: []*profilev1.Label{
						{Key: 1, Str: 2},
					},
				}},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "baz", Value: "qux"},
						{Name: "foo", Value: "bar"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{1},
						Label: []*profilev1.Label{},
					}},
				},
			},
		},
		{
			description: "has series labels, and the only sample label name overlaps with series label, creating overlapping groups",
			labels: []*typesv1.LabelPair{
				{Name: "foo", Value: "bar"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Sample: []*profilev1.Sample{
					{
						Value: []int64{1},
						Label: []*profilev1.Label{
							{Key: 1, Str: 2},
						},
					},
					{
						Value: []int64{2},
					},
				},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "foo", Value: "bar"},
					},
					samples: []*profilev1.Sample{
						{
							Value: []int64{3},
							Label: nil,
						},
					},
				},
			},
		},
		{
			description: "has series labels, samples have distinct label sets",
			labels: []*typesv1.LabelPair{
				{Name: "baz", Value: "qux"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "foo", "bar", "waldo", "fred"},
				Sample: []*profilev1.Sample{
					{
						Value: []int64{1},
						Label: []*profilev1.Label{
							{Key: 1, Str: 2},
						},
					},
					{
						Value: []int64{2},
						Label: []*profilev1.Label{
							{Key: 3, Str: 4},
						},
					},
				},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "baz", Value: "qux"},
						{Name: "foo", Value: "bar"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{1},
						Label: []*profilev1.Label{},
					}},
				},
				{
					labels: []*typesv1.LabelPair{
						{Name: "baz", Value: "qux"},
						{Name: "waldo", Value: "fred"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{2},
						Label: []*profilev1.Label{},
					}},
				},
			},
		},
		{
			description: "has series labels that should be renamed to no longer include godeltaprof",
			rules:       defaultRelabelConfigs,
			labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: "godeltaprof_memory"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{""},
				Sample: []*profilev1.Sample{{
					Value: []int64{2},
					Label: []*profilev1.Label{},
				}},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "__delta__", Value: "false"},
						{Name: "__name__", Value: "memory"},
						{Name: "__name_replaced__", Value: "godeltaprof_memory"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{2},
						Label: []*profilev1.Label{},
					}},
				},
			},
		},
		{
			description: "has series labels and sample label, which relabel rules drop",
			rules: []*relabel.Config{
				{
					Action:       relabel.Drop,
					SourceLabels: []model.LabelName{"__name__", "span_name"},
					Separator:    "/",
					Regex:        relabel.MustNewRegexp("unwanted/randomness"),
				},
			},
			labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: "unwanted"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "span_name", "randomness"},
				Sample: []*profilev1.Sample{
					{
						Value: []int64{2},
						Label: []*profilev1.Label{
							{Key: 1, Str: 2},
						},
					},
					{
						Value: []int64{1},
					},
				},
			},
			expectProfilesDropped: 0,
			expectBytesDropped:    3,
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "__name__", Value: "unwanted"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{1},
					}},
				},
			},
		},
		{
			description: "has series/sample labels, drops everything",
			rules: []*relabel.Config{
				{
					Action: relabel.Drop,
					Regex:  relabel.MustNewRegexp(".*"),
				},
			},
			labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: "unwanted"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "span_name", "randomness"},
				Sample: []*profilev1.Sample{
					{
						Value: []int64{2},
						Label: []*profilev1.Label{
							{Key: 1, Str: 2},
						},
					},
					{
						Value: []int64{1},
					},
				},
			},
			expectProfilesDropped: 1,
			expectBytesDropped:    6,
			expected:              []sampleSeries{},
		},
		{
			description: "has series labels / sample rules, drops samples label",
			rules: []*relabel.Config{
				{
					Action:      relabel.Replace,
					Regex:       relabel.MustNewRegexp(".*"),
					Replacement: "",
					TargetLabel: "span_name",
				},
			},
			labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: "unwanted"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "span_name", "randomness"},
				Sample: []*profilev1.Sample{
					{
						Value: []int64{2},
						Label: []*profilev1.Label{
							{Key: 1, Str: 2},
						},
					},
					{
						Value: []int64{1},
					},
				},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "__name__", Value: "unwanted"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{3},
					}},
				},
			},
		},
		{
			description: "ensure only samples of same stacktraces get grouped",
			labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: "profile"},
			},
			profile: &profilev1.Profile{
				StringTable: []string{"", "foo", "bar", "binary", "span_id", "aaaabbbbccccdddd", "__name__"},
				Location: []*profilev1.Location{
					{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
					{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
				},
				Mapping: []*profilev1.Mapping{{}, {Id: 1, Filename: 3}},
				Function: []*profilev1.Function{
					{Id: 1, Name: 1},
					{Id: 2, Name: 2},
				},
				Sample: []*profilev1.Sample{
					{
						LocationId: []uint64{1, 2},
						Value:      []int64{2},
						Label: []*profilev1.Label{
							// This __name__ label is expected to be removed as it overlaps with the series label name
							{Key: 6, Str: 1},
						},
					},
					{
						LocationId: []uint64{1, 2},
						Value:      []int64{1},
					},
					{
						LocationId: []uint64{1, 2},
						Value:      []int64{4},
						Label: []*profilev1.Label{
							{Key: 4, Str: 5},
						},
					},
					{
						Value: []int64{8},
					},
					{
						Value: []int64{16},
						Label: []*profilev1.Label{
							{Key: 1, Str: 2},
						},
					},
				},
			},
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{
						{Name: "__name__", Value: "profile"},
					},
					samples: []*profilev1.Sample{
						{
							LocationId: []uint64{1, 2},
							Value:      []int64{3},
						},
						{
							LocationId: []uint64{1, 2},
							Value:      []int64{4},
							Label: []*profilev1.Label{
								{Key: 4, Str: 5},
							},
						},
						{
							Value: []int64{8},
						},
					},
				},
				{
					labels: []*typesv1.LabelPair{
						{Name: "__name__", Value: "profile"},
						{Name: "foo", Value: "bar"},
					},
					samples: []*profilev1.Sample{{
						Value: []int64{16},
						Label: []*profilev1.Label{},
					}},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.description, func(t *testing.T) {
			v := new(mockVisitor)
			VisitSampleSeries(
				tc.profile,
				tc.labels,
				tc.rules,
				v,
			)

			assert.Equal(t, tc.expectBytesDropped, v.discardedBytes)
			assert.Equal(t, tc.expectProfilesDropped, v.discardedProfiles)

			if tc.expectNoSeries {
				assert.Nil(t, v.visited)
				assert.Equal(t, tc.expectLabels, v.labels)
				return
			}

			for i, actual := range v.visited {
				expected := tc.expected[i]
				assert.Equal(t, expected.labels, actual.labels)
				assert.Equal(t, expected.samples, actual.samples)
			}
		})
	}
}
