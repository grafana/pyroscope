package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

func Test_StackTraceFilter(t *testing.T) {
	profile := &googlev1.Profile{
		StringTable: []string{"", "foo", "bar", "baz", "qux"},
		Function: []*googlev1.Function{
			{Id: 1, Name: 1},
			{Id: 2, Name: 2},
			{Id: 3, Name: 3},
			{Id: 4, Name: 4},
		},
		Mapping: []*googlev1.Mapping{{Id: 1}},
		Location: []*googlev1.Location{
			{Id: 1, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 1, Line: 1}}}, // foo
			{Id: 2, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 1}}}, // bar:1
			{Id: 3, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 2, Line: 2}}}, // bar:2
			{Id: 4, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 3, Line: 1}}}, // baz
			{Id: 5, MappingId: 1, Line: []*googlev1.Line{{FunctionId: 4, Line: 1}}}, // qux
		},
		Sample: []*googlev1.Sample{
			{LocationId: []uint64{4, 2, 1}, Value: []int64{1}}, // foo, bar:1, baz
			{LocationId: []uint64{3, 1}, Value: []int64{1}},    // foo, bar:2
			{LocationId: []uint64{4, 1}, Value: []int64{1}},    // foo, baz
			{LocationId: []uint64{5}, Value: []int64{1}},       // qux

			{LocationId: []uint64{2}, Value: []int64{1}},    // bar:1
			{LocationId: []uint64{1, 2}, Value: []int64{1}}, // bar:1, foo
			{LocationId: []uint64{3}, Value: []int64{1}},    // bar:2
			{LocationId: []uint64{1, 3}, Value: []int64{1}}, // bar:2, foo
		},
	}

	db := NewSymDB(DefaultConfig().WithDirectory(t.TempDir()))
	w := db.WriteProfileSymbols(0, profile)

	p, err := db.Partition(context.Background(), 0)
	require.NoError(t, err)
	symbols := p.Symbols()

	type testCase struct {
		selector *typesv1.StackTraceSelector
		expected CallSiteValues
	}

	testCases := []testCase{
		{
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{Name: "foo"}},
			},
			expected: CallSiteValues{
				Flat:          0,
				Total:         3,
				LocationFlat:  2,
				LocationTotal: 5,
			},
		},
		{
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{Name: "bar"}},
			},
			expected: CallSiteValues{
				Flat:          2,
				Total:         4,
				LocationFlat:  3,
				LocationTotal: 6,
			},
		},
		{
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{Name: "foo"}, {Name: "bar"}},
			},
			expected: CallSiteValues{
				Flat:          1,
				Total:         2,
				LocationFlat:  3,
				LocationTotal: 6,
			},
		},
		{
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{Name: "foo"}, {Name: "bar"}, {Name: "baz"}},
			},
			expected: CallSiteValues{
				Flat:          1,
				Total:         1,
				LocationFlat:  2,
				LocationTotal: 2,
			},
		},
		{
			selector: &typesv1.StackTraceSelector{
				CallSite: []*typesv1.Location{{Name: "foo"}, {Name: "bar"}, {Name: "baz"}, {Name: "qux"}},
			},
			expected: CallSiteValues{
				Flat:          0,
				Total:         0,
				LocationFlat:  1,
				LocationTotal: 1,
			},
		},
		{selector: &typesv1.StackTraceSelector{}},
		{},
	}

	for _, tc := range testCases {
		selection := SelectStackTraces(symbols, tc.selector)
		var values CallSiteValues
		selection.CallSiteValues(&values, w[0].Samples)
		assert.Equal(t, tc.expected, values, "selector: %+v", tc.selector)
	}
}

func Benchmark_StackTraceFilter(b *testing.B) {
	s := memSuite{t: b, files: [][]string{{"testdata/big-profile.pb.gz"}}}
	s.config = DefaultConfig().WithDirectory(b.TempDir())
	s.init()
	samples := s.indexed[0][0].Samples

	prt, err := s.db.Partition(context.Background(), 0)
	require.NoError(b, err)
	symbols := prt.Symbols()

	p := s.profiles[0]
	stack := p.Sample[len(p.Sample)/3].LocationId
	selector := buildStackTraceSelector(p, stack[len(stack)/10:])
	var values CallSiteValues

	b.ReportAllocs()
	b.ResetTimer()

	selection := SelectStackTraces(symbols, selector)
	for i := 0; i < b.N; i++ {
		selection.CallSiteValues(&values, samples)
	}
}

func buildStackTraceSelector(p *googlev1.Profile, locs []uint64) *typesv1.StackTraceSelector {
	var selector typesv1.StackTraceSelector
	for _, n := range locs {
		for _, l := range p.Location[n-1].Line {
			fn := p.Function[l.FunctionId-1]
			selector.CallSite = append(selector.CallSite, &typesv1.Location{
				Name: p.StringTable[fn.Name],
			})
		}
	}
	slices.Reverse(selector.CallSite)
	return &selector
}
