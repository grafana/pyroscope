package symdb

import (
	"context"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_memory_Resolver_ResolvePprof(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Pprof()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, pprofFingerprint(resolved, 0))
}

func Test_block_Resolver_ResolvePprof_multiple_partitions(t *testing.T) {
	s := newBlockSuite(t, [][]string{
		{"testdata/profile.pb.gz"},
		{"testdata/profile.pb.gz"},
	})
	defer s.teardown()
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	for i := range expectedFingerprint {
		expectedFingerprint[i][1] *= 2
	}
	r := NewResolver(context.Background(), s.reader)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	r.AddSamples(1, s.indexed[1][0].Samples)
	resolved, err := r.Pprof()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, pprofFingerprint(resolved, 0))
}

func Benchmark_Resolver_ResolvePprof_Small(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolvePprof(s.db, samples, 0))
	b.Run("1K", benchmarkResolverResolvePprof(s.db, samples, 1<<10))
	b.Run("8K", benchmarkResolverResolvePprof(s.db, samples, 8<<10))
}

func Benchmark_Resolver_ResolvePprof_Big(b *testing.B) {
	s := memSuite{t: b, files: [][]string{{"testdata/big-profile.pb.gz"}}}
	s.config = DefaultConfig().WithDirectory(b.TempDir())
	s.init()
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolvePprof(s.db, samples, 0))
	b.Run("8K", benchmarkResolverResolvePprof(s.db, samples, 8<<10))
	b.Run("16K", benchmarkResolverResolvePprof(s.db, samples, 16<<10))
	b.Run("32K", benchmarkResolverResolvePprof(s.db, samples, 32<<10))
	b.Run("64K", benchmarkResolverResolvePprof(s.db, samples, 64<<10))
}

func benchmarkResolverResolvePprof(sym SymbolsReader, samples v1.Samples, n int64) func(b *testing.B) {
	return func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := NewResolver(context.Background(), sym, WithResolverMaxNodes(n))
			r.AddSamples(0, samples)
			_, _ = r.Pprof()
		}
	}
}

func Test_Pprof_subtree(t *testing.T) {
	profile := &googlev1.Profile{
		StringTable: []string{"", "a", "b", "c", "d"},
		Function: []*googlev1.Function{
			{Id: 1, Name: 1},
			{Id: 2, Name: 2},
			{Id: 3, Name: 3},
			{Id: 4, Name: 4},
		},
		Mapping: []*googlev1.Mapping{{Id: 1}},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 1, Line: 1}}}, // a
			{Id: 2, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 1}}}, // b:1
			{Id: 3, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 2}}}, // b:2
			{Id: 4, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 3, Line: 1}}}, // c
			{Id: 5, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 4, Line: 1}}}, // d
		},
		Sample: []*googlev1.Sample{
			{LocationId: []uint64{4, 2, 1}, Value: []int64{1}}, // a, b:1, c
			{LocationId: []uint64{3, 1}, Value: []int64{1}},    // a, b:2
			{LocationId: []uint64{4, 1}, Value: []int64{1}},    // a, c
			{LocationId: []uint64{5}, Value: []int64{1}},       // d
		},
	}

	db := NewSymDB(DefaultConfig().WithDirectory(t.TempDir()))
	w := db.WriteProfileSymbols(0, profile)
	r := NewResolver(context.Background(), db,
		WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
			CallSite: []*typesv1.Location{{Name: "a"}, {Name: "b"}},
		}))

	r.AddSamples(0, w[0].Samples)
	actual, err := r.Pprof()
	require.NoError(t, err)
	// Sample order is not deterministic.
	sort.Slice(actual.Sample, func(i, j int) bool {
		return slices.Compare(actual.Sample[i].LocationId, actual.Sample[j].LocationId) >= 0
	})

	expected := &googlev1.Profile{
		PeriodType:  &googlev1.ValueType{},
		SampleType:  []*googlev1.ValueType{{}},
		StringTable: []string{"", "a", "b", "c"},
		Function: []*googlev1.Function{
			{Id: 1, Name: 1},
			{Id: 2, Name: 2},
			{Id: 3, Name: 3},
		},
		Mapping: []*googlev1.Mapping{{Id: 1}},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 1, Line: 1}}}, // a
			{Id: 2, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 1}}}, // b:1
			{Id: 3, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 2}}}, // b:2
			{Id: 4, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 3, Line: 1}}}, // c
		},
		Sample: []*googlev1.Sample{
			{LocationId: []uint64{4, 2, 1}, Value: []int64{1}}, // a, b:1, c
			{LocationId: []uint64{3, 1}, Value: []int64{1}},    // a, b:2
		},
	}

	require.Equal(t, expected, actual)
}

func Test_Pprof_subtree_multiple_versions(t *testing.T) {
	profile := &googlev1.Profile{
		StringTable: []string{"", "a", "b", "c", "d"},
		Function: []*googlev1.Function{
			{Id: 1, Name: 1},               // a
			{Id: 2, Name: 2},               // b
			{Id: 3, Name: 3},               // c
			{Id: 4, Name: 4, StartLine: 1}, // d
			{Id: 5, Name: 4, StartLine: 2}, // d(2)
		},
		Mapping: []*googlev1.Mapping{{Id: 1}},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 1, Line: 1}}}, // a
			{Id: 2, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 1}}}, // b:1
			{Id: 3, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 2}}}, // b:2
			{Id: 4, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 3, Line: 1}}}, // c
			{Id: 5, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 4, Line: 1}}}, // d
			{Id: 6, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 5, Line: 1}}}, // d(2)
		},
		Sample: []*googlev1.Sample{
			{LocationId: []uint64{5, 4, 2, 1}, Value: []int64{1}}, // a, b:1, c, d
			{LocationId: []uint64{6, 4, 3, 1}, Value: []int64{1}}, // a, b:2, c, d(2)
			{LocationId: []uint64{3, 1}, Value: []int64{1}},       // a, b:2
			{LocationId: []uint64{4, 1}, Value: []int64{1}},       // a, c
			{LocationId: []uint64{5}, Value: []int64{1}},          // d
			{LocationId: []uint64{6}, Value: []int64{1}},          // d (2)
		},
	}

	db := NewSymDB(DefaultConfig().WithDirectory(t.TempDir()))
	w := db.WriteProfileSymbols(0, profile)
	r := NewResolver(context.Background(), db,
		WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
			CallSite: []*typesv1.Location{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}},
		}))

	r.AddSamples(0, w[0].Samples)
	actual, err := r.Pprof()
	require.NoError(t, err)
	// Sample order is not deterministic.
	sort.Slice(actual.Sample, func(i, j int) bool {
		return slices.Compare(actual.Sample[i].LocationId, actual.Sample[j].LocationId) >= 0
	})

	expected := &googlev1.Profile{
		PeriodType:  &googlev1.ValueType{},
		SampleType:  []*googlev1.ValueType{{}},
		StringTable: []string{"", "a", "b", "c", "d"},
		Function: []*googlev1.Function{
			{Id: 1, Name: 1},               // a
			{Id: 2, Name: 2},               // b
			{Id: 3, Name: 3},               // c
			{Id: 4, Name: 4, StartLine: 1}, // d
			{Id: 5, Name: 4, StartLine: 2}, // d(2)
		},
		Mapping: []*googlev1.Mapping{{Id: 1}},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 1, Line: 1}}}, // a
			{Id: 2, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 1}}}, // b:1
			{Id: 3, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 2}}}, // b:2
			{Id: 4, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 3, Line: 1}}}, // c
			{Id: 5, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 4, Line: 1}}}, // d
			{Id: 6, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 5, Line: 1}}}, // d(2)
		},
		Sample: []*googlev1.Sample{
			{LocationId: []uint64{6, 4, 3, 1}, Value: []int64{1}}, // a, b:2, c, d(2)
			{LocationId: []uint64{5, 4, 2, 1}, Value: []int64{1}}, // a, b:1, c, d
		},
	}

	require.Equal(t, expected, actual)
}

func Test_Resolver_pprof_options(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	const samplesTotal = 561

	var sc PartitionStats
	s.db.partitions[0].WriteStats(&sc)
	t.Logf("%#v\n", sc)

	type testCase struct {
		name     string
		expected int
		options  []ResolverOption
	}

	testCases := []testCase{
		{
			name:     "no options",
			expected: samplesTotal,
		},
		{
			name:     "0 max nodes",
			expected: samplesTotal,
			options: []ResolverOption{
				WithResolverMaxNodes(0),
			},
		},
		{
			name:     "10 max nodes",
			expected: 22,
			options: []ResolverOption{
				WithResolverMaxNodes(10),
			},
		},

		{
			name:     "callSite",
			expected: 54,
			options: []ResolverOption{
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					CallSite: []*typesv1.Location{{Name: "runtime.main"}},
				}),
			},
		},
		{
			name:     "callSite 10 max nodes",
			expected: 14,
			options: []ResolverOption{
				WithResolverMaxNodes(10),
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					CallSite: []*typesv1.Location{{Name: "runtime.main"}},
				}),
			},
		},
		{
			name:     "nil StackTraceSelector",
			expected: samplesTotal,
			options: []ResolverOption{
				WithResolverStackTraceSelector(nil),
			},
		},
		{
			name:     "nil StackTraceSelector 10 max nodes",
			expected: 22,
			options: []ResolverOption{
				WithResolverMaxNodes(10),
				WithResolverStackTraceSelector(nil),
			},
		},
		{
			name:     "empty StackTraceSelector.CallSite",
			expected: samplesTotal,
			options: []ResolverOption{
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					CallSite: []*typesv1.Location{},
				}),
			},
		},
		{
			name:     "StackTraceSelector GoPGO empty",
			expected: samplesTotal,
			options: []ResolverOption{
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					GoPgo: &typesv1.GoPGO{},
				}),
			},
		},
		{
			name:     "StackTraceSelector GoPGO takes precedence",
			expected: 414,
			options: []ResolverOption{
				WithResolverMaxNodes(10),
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					CallSite: []*typesv1.Location{{Name: "runtime.main"}},
					GoPgo: &typesv1.GoPGO{
						KeepLocations: 5,
					},
				}),
			},
		},
		{
			name:     "GoPGO KeepLocations 5",
			expected: 414,
			options: []ResolverOption{
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					GoPgo: &typesv1.GoPGO{
						KeepLocations: 5,
					},
				}),
			},
		},
		{
			name:     "GoPGO AggregateCallees",
			expected: 442,
			options: []ResolverOption{
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					GoPgo: &typesv1.GoPGO{
						AggregateCallees: true,
					},
				}),
			},
		},
		{
			name:     "GoPGO AggregateCallees KeepLocations 5",
			expected: 316,
			options: []ResolverOption{
				WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
					GoPgo: &typesv1.GoPGO{
						KeepLocations:    5,
						AggregateCallees: true,
					},
				}),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := NewResolver(context.Background(), s.db, tc.options...)
			defer r.Release()
			r.AddSamples(0, samples)
			p, err := r.Pprof()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, len(p.Sample))

			var sum int64
			for _, x := range p.Sample {
				sum += x.Value[0]
			}
		})
	}
}

// The test examines how strings are copied from the Symbols
// to the Profile at resolve.
//
// We used to have an issue that the first string is not empty,
// as it's required by the pprof format:
// https://github.com/grafana/pyroscope/issues/3199
func Test_Resolver_pprof_strings(t *testing.T) {
	type testCase struct {
		name     string
		symbols  []string
		profile  *googlev1.Profile
		expected *googlev1.Profile
	}

	testCases := []testCase{
		{
			name:    "normal_sparse",
			symbols: []string{"", "foo", "baz", "bar"},
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  0, // ""
				}},
				Function: []*googlev1.Function{{
					Name:       3, // bar
					SystemName: 3,
					Filename:   3,
				}},
			},
			expected: &googlev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  0,
				}},
				Function: []*googlev1.Function{{
					Name:       2, // bar
					SystemName: 2,
					Filename:   2,
				}},
			},
		},
		{
			name:    "normal_dense",
			symbols: []string{"", "foo", "bar"},
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  0, // ""
				}},
				Function: []*googlev1.Function{{
					Name:       2, // bar
					SystemName: 2,
					Filename:   2,
				}},
			},
			expected: &googlev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  0,
				}},
				Function: []*googlev1.Function{{
					Name:       2, // bar
					SystemName: 2,
					Filename:   2,
				}},
			},
		},
		{
			name:    "no_zero_sparse",
			symbols: []string{"foo", "baz", "fred", "bar"},
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Filename: 0, // foo
					BuildId:  1, // baz
				}},
				Function: []*googlev1.Function{{
					Name:       3, // bar
					SystemName: 3,
					Filename:   3,
				}},
			},
			expected: &googlev1.Profile{
				StringTable: []string{"", "foo", "baz", "bar"},
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  2, // baz
				}},
				Function: []*googlev1.Function{{
					Name:       3, // bar
					SystemName: 3,
					Filename:   3,
				}},
			},
		},
		{
			name:    "no_zero_dense",
			symbols: []string{"foo", "baz", "bar"},
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Filename: 0, // foo
					BuildId:  1, // baz
				}},
				Function: []*googlev1.Function{{
					Name:       2, // bar
					SystemName: 2,
					Filename:   2,
				}},
			},
			expected: &googlev1.Profile{
				StringTable: []string{"", "foo", "baz", "bar"},
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  2, // baz
				}},
				Function: []*googlev1.Function{{
					Name:       3, // bar
					SystemName: 3,
					Filename:   3,
				}},
			},
		},
		{
			name:    "unordered_dense",
			symbols: []string{"foo", "baz", "", "bar"},
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Filename: 0, // foo
					BuildId:  2, // ""
				}},
				Function: []*googlev1.Function{{
					Name:       3, // bar
					SystemName: 3,
					Filename:   3,
				}},
			},
			expected: &googlev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  0, //
				}},
				Function: []*googlev1.Function{{
					Name:       2, // bar
					SystemName: 2,
					Filename:   2,
				}},
			},
		},
		{
			name:    "unordered_sparse",
			symbols: []string{"foo", "fred", "baz", "", "bar"},
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					Filename: 0, // foo
					BuildId:  3, // ""
				}},
				Function: []*googlev1.Function{{
					Name:       4, // bar
					SystemName: 4,
					Filename:   4,
				}},
			},
			expected: &googlev1.Profile{
				StringTable: []string{"", "foo", "bar"},
				Mapping: []*googlev1.Mapping{{
					Filename: 1, // foo
					BuildId:  0, //
				}},
				Function: []*googlev1.Function{{
					Name:       2, // bar
					SystemName: 2,
					Filename:   2,
				}},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := &Symbols{Strings: tc.symbols}
			copyStrings(tc.profile, s, nil)
			require.Equal(t, tc.expected, tc.profile)
		})
	}
}
