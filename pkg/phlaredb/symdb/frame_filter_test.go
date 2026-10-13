package symdb

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestFrameFilterSelectsWholeSamples(t *testing.T) {
	profile := &profilev1.Profile{
		StringTable: []string{"", "root", "a", "b", "forbidden"},
		Function: []*profilev1.Function{
			{Id: 1, Name: 1}, {Id: 2, Name: 2}, {Id: 3, Name: 3}, {Id: 4, Name: 4},
		},
		Mapping: []*profilev1.Mapping{{Id: 1}},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
			{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
			{Id: 3, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 3}}},
			{Id: 4, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 4}}},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{1}},
			{LocationId: []uint64{3, 2, 1}, Value: []int64{1}},
			{LocationId: []uint64{3, 1}, Value: []int64{1}},
			{LocationId: []uint64{4, 1}, Value: []int64{1}},
			{LocationId: []uint64{4, 2, 1}, Value: []int64{1}},
		},
	}
	db := NewSymDB(DefaultConfig().WithDirectory(t.TempDir()))
	indexed := db.WriteProfileSymbols(0, profile)

	for _, tc := range []struct {
		name    string
		filter  *typesv1.StackFrameFilter
		wantSum int64
	}{
		{"include", &typesv1.StackFrameFilter{IncludeFunctionNames: []string{"a"}}, 3},
		{"all included", &typesv1.StackFrameFilter{IncludeFunctionNames: []string{"a", "b"}}, 1},
		{"exclude", &typesv1.StackFrameFilter{ExcludeFunctionNames: []string{"forbidden"}}, 3},
		{"include and exclude", &typesv1.StackFrameFilter{IncludeFunctionNames: []string{"a"}, ExcludeFunctionNames: []string{"forbidden"}}, 2},
		{"missing include", &typesv1.StackFrameFilter{IncludeFunctionNames: []string{"missing"}}, 0},
		{"regex include", &typesv1.StackFrameFilter{IncludeFunctionNameRegexes: []string{"^a$"}}, 3},
		{"regex exclude substring", &typesv1.StackFrameFilter{ExcludeFunctionNameRegexes: []string{"forbid"}}, 3},
		{"all regex includes", &typesv1.StackFrameFilter{IncludeFunctionNameRegexes: []string{"^a$", "^b$"}}, 1},
		{"exact and regex include", &typesv1.StackFrameFilter{IncludeFunctionNames: []string{"a"}, IncludeFunctionNameRegexes: []string{"^b$"}}, 1},
		{"same frame satisfies both", &typesv1.StackFrameFilter{IncludeFunctionNames: []string{"a"}, IncludeFunctionNameRegexes: []string{"^a$"}}, 3},
		{"regex include and exclude", &typesv1.StackFrameFilter{IncludeFunctionNameRegexes: []string{"^a$"}, ExcludeFunctionNameRegexes: []string{"^forbidden$"}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selector := &typesv1.StackTraceSelector{FrameFilter: tc.filter}
			resolver := NewResolver(context.Background(), db, WithResolverStackTraceSelector(selector))
			resolver.AddSamples(0, indexed[0].Samples)
			got, err := resolver.Pprof()
			require.NoError(t, err)
			var total int64
			for _, sample := range got.Sample {
				total += sample.Value[0]
			}
			require.Equal(t, tc.wantSum, total)
			resolver.Release()

			resolver = NewResolver(context.Background(), db, WithResolverStackTraceSelector(selector))
			resolver.AddSamples(0, indexed[0].Samples)
			tree, err := resolver.Tree()
			require.NoError(t, err)
			require.Equal(t, tc.wantSum, tree.Total())
			resolver.Release()
		})
	}

	// Prefix selection and frame selection both apply to the same full sample.
	selector := &typesv1.StackTraceSelector{
		CallSite: []*typesv1.Location{{Name: "root"}, {Name: "a"}},
		FrameFilter: &typesv1.StackFrameFilter{
			IncludeFunctionNames: []string{"b"},
		},
	}
	r := NewResolver(context.Background(), db, WithResolverStackTraceSelector(selector))
	r.AddSamples(0, indexed[0].Samples)
	tree, err := r.Tree()
	require.NoError(t, err)
	require.Equal(t, int64(1), tree.Total())
	r.Release()
}

func TestFrameFilterRejectsInvalidRegex(t *testing.T) {
	filter := &typesv1.StackFrameFilter{IncludeFunctionNameRegexes: []string{"["}}
	require.ErrorContains(t, ValidateFrameFilter(filter), "invalid include function name regex")
	_, err := NewFrameMatcher(&Symbols{}, filter)
	require.ErrorContains(t, err, "invalid include function name regex")

	filter = &typesv1.StackFrameFilter{ExcludeFunctionNameRegexes: []string{"("}}
	require.ErrorContains(t, ValidateFrameFilter(filter), "invalid exclude function name regex")
	_, err = NewFrameMatcher(&Symbols{}, filter)
	require.ErrorContains(t, err, "invalid exclude function name regex")
}

func BenchmarkFrameFilterResolver(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/big-profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	var frame string
	for _, fn := range s.profiles[0].Function {
		if name := s.profiles[0].StringTable[fn.Name]; name != "" {
			frame = name
			break
		}
	}
	if frame == "" {
		b.Fatal("fixture has no function names")
	}
	for _, tc := range []struct {
		name     string
		selector *typesv1.StackTraceSelector
	}{
		{"Unfiltered", nil},
		{"IncludeFrame", &typesv1.StackTraceSelector{FrameFilter: &typesv1.StackFrameFilter{IncludeFunctionNames: []string{frame}}}},
		{"ExcludeFrame", &typesv1.StackTraceSelector{FrameFilter: &typesv1.StackFrameFilter{ExcludeFunctionNames: []string{frame}}}},
		{"IncludeRegex", &typesv1.StackTraceSelector{FrameFilter: &typesv1.StackFrameFilter{IncludeFunctionNameRegexes: []string{"^" + regexp.QuoteMeta(frame) + "$"}}}},
		{"ExcludeRegex", &typesv1.StackTraceSelector{FrameFilter: &typesv1.StackFrameFilter{ExcludeFunctionNameRegexes: []string{"^" + regexp.QuoteMeta(frame) + "$"}}}},
	} {
		for _, format := range []string{"Tree", "Pprof"} {
			b.Run(format+"/"+tc.name, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					r := NewResolver(context.Background(), s.db, WithResolverMaxNodes(8<<10), WithResolverStackTraceSelector(tc.selector))
					r.AddSamples(0, samples)
					var err error
					if format == "Tree" {
						_, err = r.Tree()
					} else {
						_, err = r.Pprof()
					}
					r.Release()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
