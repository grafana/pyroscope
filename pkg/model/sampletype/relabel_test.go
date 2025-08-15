package sampletype

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/assert"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/validation"
)

func TestRelabelProfile(t *testing.T) {
	tests := []struct {
		name           string
		profile        *googlev1.Profile
		rules          []*relabel.Config
		expectedTypes  []string
		expectedValues [][]int64
	}{
		{
			name: "drop alloc_objects and alloc_space from memory profile",
			profile: &googlev1.Profile{
				StringTable: []string{"", "alloc_objects", "count", "alloc_space", "bytes", "inuse_objects", "inuse_space"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2}, // alloc_objects, count
					{Type: 3, Unit: 4}, // alloc_space, bytes
					{Type: 5, Unit: 2}, // inuse_objects, count
					{Type: 6, Unit: 4}, // inuse_space, bytes
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{100, 2048, 50, 1024}},
					{LocationId: []uint64{2}, Value: []int64{200, 4096, 150, 3072}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
					{Id: 2, MappingId: 1, Address: 0xcafe000},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__type__"},
					Regex:        relabel.MustNewRegexp("alloc_.*"),
					Action:       relabel.Drop,
				},
			},
			expectedTypes: []string{"inuse_objects", "inuse_space"},
			expectedValues: [][]int64{
				{50, 1024},
				{150, 3072},
			},
		},
		{
			name: "keep only inuse_space",
			profile: &googlev1.Profile{
				StringTable: []string{"", "alloc_objects", "count", "alloc_space", "bytes", "inuse_objects", "inuse_space"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2}, // alloc_objects, count
					{Type: 3, Unit: 4}, // alloc_space, bytes
					{Type: 5, Unit: 2}, // inuse_objects, count
					{Type: 6, Unit: 4}, // inuse_space, bytes
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{100, 2048, 50, 1024}},
					{LocationId: []uint64{2}, Value: []int64{200, 4096, 150, 3072}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
					{Id: 2, MappingId: 1, Address: 0xcafe000},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__type__"},
					Regex:        relabel.MustNewRegexp("inuse_space"),
					Action:       relabel.Keep,
				},
			},
			expectedTypes: []string{"inuse_space"},
			expectedValues: [][]int64{
				{1024},
				{3072},
			},
		},
		{
			name: "drop by unit - drop count types",
			profile: &googlev1.Profile{
				StringTable: []string{"", "alloc_objects", "count", "alloc_space", "bytes", "inuse_objects", "inuse_space"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2}, // alloc_objects, count
					{Type: 3, Unit: 4}, // alloc_space, bytes
					{Type: 5, Unit: 2}, // inuse_objects, count
					{Type: 6, Unit: 4}, // inuse_space, bytes
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{100, 2048, 50, 1024}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__unit__"},
					Regex:        relabel.MustNewRegexp("count"),
					Action:       relabel.Drop,
				},
			},
			expectedTypes: []string{"alloc_space", "inuse_space"},
			expectedValues: [][]int64{
				{2048, 1024},
			},
		},
		{
			name: "drop all sample types",
			profile: &googlev1.Profile{
				StringTable: []string{"", "cpu", "nanoseconds"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2}, // cpu, nanoseconds
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{1000}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__type__"},
					Regex:        relabel.MustNewRegexp(".*"),
					Action:       relabel.Drop,
				},
			},
			expectedTypes:  []string{},
			expectedValues: [][]int64{},
		},
		{
			name: "no rules - no changes",
			profile: &googlev1.Profile{
				StringTable: []string{"", "cpu", "nanoseconds"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2},
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{1000}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules:         []*relabel.Config{},
			expectedTypes: []string{"cpu"},
			expectedValues: [][]int64{
				{1000},
			},
		},
		{
			name: "complex relabeling with multiple rules",
			profile: &googlev1.Profile{
				StringTable: []string{"", "samples", "count", "cpu", "nanoseconds", "wall", "goroutines"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2}, // samples, count
					{Type: 3, Unit: 4}, // cpu, nanoseconds
					{Type: 5, Unit: 4}, // wall, nanoseconds
					{Type: 6, Unit: 2}, // goroutines, count
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{10, 1000000, 2000000, 5}},
					{LocationId: []uint64{2}, Value: []int64{20, 3000000, 4000000, 8}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
					{Id: 2, MappingId: 1, Address: 0xcafe000},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__type__"},
					Regex:        relabel.MustNewRegexp("samples"),
					Action:       relabel.Drop,
				},
				{
					SourceLabels: []model.LabelName{"__type__", "__unit__"},
					Separator:    "/",
					Regex:        relabel.MustNewRegexp("goroutines/count"),
					Action:       relabel.Drop,
				},
			},
			expectedTypes: []string{"cpu", "wall"},
			expectedValues: [][]int64{
				{1000000, 2000000},
				{3000000, 4000000},
			},
		},
		{
			name: "keep rule with no matches drops everything",
			profile: &googlev1.Profile{
				StringTable: []string{"", "cpu", "nanoseconds", "wall"},
				SampleType: []*googlev1.ValueType{
					{Type: 1, Unit: 2}, // cpu, nanoseconds
					{Type: 3, Unit: 2}, // wall, nanoseconds
				},
				Sample: []*googlev1.Sample{
					{LocationId: []uint64{1}, Value: []int64{1000, 2000}},
				},
				Location: []*googlev1.Location{
					{Id: 1, MappingId: 1, Address: 0xef},
				},
				Mapping: []*googlev1.Mapping{{Id: 1}},
			},
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__type__"},
					Regex:        relabel.MustNewRegexp("memory"),
					Action:       relabel.Keep,
				},
			},
			expectedTypes:  []string{},
			expectedValues: [][]int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := func(t testing.TB) {
				assert.Equal(t, len(tt.expectedTypes), len(tt.profile.SampleType), "sample type count mismatch")
				for i, expectedType := range tt.expectedTypes {
					actualType := tt.profile.StringTable[tt.profile.SampleType[i].Type]
					assert.Equal(t, expectedType, actualType, "sample type at index %d", i)
				}
				if tt.expectedValues != nil {
					assert.Equal(t, len(tt.expectedValues), len(tt.profile.Sample), "sample count mismatch")
					for i, sample := range tt.profile.Sample {
						if i < len(tt.expectedValues) {
							assert.Equal(t, tt.expectedValues[i], sample.Value, "sample values at index %d", i)
						}
					}
				}
			}

			p := validation.ValidatedProfile{Profile: pprof.RawFromProto(tt.profile)}

			Relabel(p, tt.rules, nil)

			p.Normalize()
			check(t)
		})
	}
}

func TestTestdata(t *testing.T) {
	tests := []struct {
		f                      string
		rules                  []*relabel.Config
		series                 phlaremodel.Labels
		expectedSize           int
		expectedNormalizedSize int
	}{
		{
			f: "../../../pkg/pprof/testdata/heap",
			rules: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__type__", "service_name"},
					Separator:    ";",
					Regex:        relabel.MustNewRegexp("inuse_space;test_service_name"),
					Action:       relabel.Keep,
				},
			},
			series: []*typesv1.LabelPair{{
				Name:  "service_name",
				Value: "test_service_name",
			}},
			expectedSize:           847138,
			expectedNormalizedSize: 46178,
		},
	}
	for _, td := range tests {
		t.Run(td.f, func(t *testing.T) {
			f, err := pprof.OpenFile(td.f)
			require.NoError(t, err)
			require.Equal(t, td.expectedSize, f.SizeVT())
			Relabel(validation.ValidatedProfile{Profile: f}, td.rules, td.series)
			f.Normalize()
			require.Equal(t, td.expectedNormalizedSize, f.SizeVT())
		})
	}
}
