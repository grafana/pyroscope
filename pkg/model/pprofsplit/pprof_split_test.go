package pprofsplit

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/validation"
)

type sampleSeries struct {
	labels  []*typesv1.LabelPair
	samples []*profilev1.Sample
}

type mockVisitor struct {
	profile           phlaremodel.Labels
	series            []sampleSeries
	discardedBytes    int
	discardedProfiles int
	err               error
}

func (m *mockVisitor) VisitProfile(labels phlaremodel.Labels) {
	m.profile = labels
}

func (m *mockVisitor) VisitSampleSeries(labels phlaremodel.Labels, samples []*profilev1.Sample) {
	m.series = append(m.series, sampleSeries{
		labels:  labels,
		samples: samples,
	})
}

func (m *mockVisitor) ValidateLabels(phlaremodel.Labels) error { return m.err }

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
		expectLabels   phlaremodel.Labels

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
			description: "does not drop samples when a label is dropped",
			rules: []*relabel.Config{
				{
					Action: relabel.LabelDrop,
					Regex:  relabel.MustNewRegexp("^label_to_drop$"),
				},
			},
			labels: []*typesv1.LabelPair{},
			profile: &profilev1.Profile{
				StringTable: []string{"", "label_to_drop", "value_1", "value_2"},
				Sample: []*profilev1.Sample{
					{
						LocationId: []uint64{1, 2},
						Value:      []int64{2},
						Label:      []*profilev1.Label{{Key: 1, Str: 2}},
					},
					{
						LocationId: []uint64{1, 3},
						Value:      []int64{2},
						Label:      []*profilev1.Label{{Key: 1, Str: 2}},
					},
					{
						LocationId: []uint64{1, 3},
						Value:      []int64{2},
						Label:      []*profilev1.Label{{Key: 1, Str: 3}}, // will get merged with the previous one
					},
					{
						LocationId: []uint64{1, 4},
						Value:      []int64{2},
						Label:      []*profilev1.Label{{Key: 1, Str: 3}},
					},
				},
			},
			expectProfilesDropped: 0,
			expectBytesDropped:    0,
			expected: []sampleSeries{
				{
					labels: []*typesv1.LabelPair{},
					samples: []*profilev1.Sample{
						{
							LocationId: []uint64{1, 2},
							Label:      []*profilev1.Label{},
							Value:      []int64{2},
						},
						{
							LocationId: []uint64{1, 3},
							Label:      []*profilev1.Label{},
							Value:      []int64{4},
						},
						{
							LocationId: []uint64{1, 4},
							Label:      []*profilev1.Label{},
							Value:      []int64{2},
						},
					},
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
			require.NoError(t, VisitSampleSeries(tc.profile, tc.labels, tc.rules, v))
			assert.Equal(t, tc.expectBytesDropped, v.discardedBytes)
			assert.Equal(t, tc.expectProfilesDropped, v.discardedProfiles)

			if tc.expectNoSeries {
				assert.Nil(t, v.series)
				assert.Equal(t, tc.expectLabels, v.profile)
				return
			}

			for i, actual := range v.series {
				expected := tc.expected[i]
				assert.Equal(t, expected.labels, actual.labels)
				assert.Equal(t, expected.samples, actual.samples)
			}
		})
	}
}

func Benchmark_VisitSampleSeries_HighCardinality(b *testing.B) {
	defaultRelabelConfigs := validation.MockDefaultOverrides().IngestionRelabelingRules("")
	defaultRelabelConfigs = append(defaultRelabelConfigs, &relabel.Config{
		Action: relabel.LabelDrop,
		Regex:  relabel.MustNewRegexp("^high_cardinality_label$"),
	})

	stringTable := []string{"", "foo", "bar", "binary", "span_id", "aaaabbbbccccdddd", "high_cardinality_label"}
	highCardinalityOffset := int64(len(stringTable))
	for i := 0; i < 10000; i++ {
		stringTable = append(stringTable, fmt.Sprintf("value_%d", i))
	}

	profile := &profilev1.Profile{
		StringTable: stringTable,
		Location:    []*profilev1.Location{{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
		Mapping:     []*profilev1.Mapping{{}, {Id: 1, Filename: 3}},
		Function:    []*profilev1.Function{{Id: 1, Name: 1}},
	}

	for i := 0; i < 30000; i++ {
		labelValue := highCardinalityOffset + int64(i/10)
		if rand.Float64() < 0.3 {
			labelValue = highCardinalityOffset - 2 // lower the cardinality to create large groups
		}
		labels := []*profilev1.Label{
			{Key: highCardinalityOffset - 1, Str: labelValue},
		}
		profile.Sample = append(profile.Sample, &profilev1.Sample{
			LocationId: []uint64{uint64(i + 1)},
			Value:      []int64{2},
			Label:      labels,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		visitor := new(mockVisitor)
		err := VisitSampleSeries(profile, []*typesv1.LabelPair{
			{Name: "__name__", Value: "profile"},
			{Name: "foo", Value: "bar"},
		}, defaultRelabelConfigs, visitor)
		require.NoError(b, err)
	}
}
