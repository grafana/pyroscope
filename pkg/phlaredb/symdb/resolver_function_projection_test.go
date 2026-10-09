package symdb

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestFunctionTableSymbolsDeduplicatesNamesAndInlineRecursion(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"main", "F"},
		Functions: []schemav1.InMemoryFunction{{Name: 0}, {Name: 1}, {Name: 1}},
		Locations: []schemav1.InMemoryLocation{
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 1}, {FunctionId: 2}},
			},
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 0}},
			},
		},
	}
	r := newFunctionTableSymbols(symbols, schemav1.Samples{
		Values: []uint64{5, 7},
	}, nil)
	r.InsertStacktrace(0, []int32{0, 1})
	r.InsertStacktrace(1, []int32{1})
	merger := model.NewFunctionTableMerger()
	merger.Merge(r.table())
	table := merger.Table()
	require.Equal(t, int64(12), table.Total)
	require.Equal(t, int64(2), table.TotalFunctions)
	require.Equal(t, "main", table.Functions[0].Name)
	require.Equal(t, int64(7), table.Functions[0].Self)
	require.Equal(t, int64(12), table.Functions[0].Total)
	require.Equal(t, "F", table.Functions[1].Name)
	require.Equal(t, int64(5), table.Functions[1].Self)
	require.Equal(t, int64(5), table.Functions[1].Total)
}

func TestFunctionTableSymbolsPrefix(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"main", "F", "elsewhere"},
		Functions: []schemav1.InMemoryFunction{{Name: 0}, {Name: 1}, {Name: 1}, {Name: 2}},
		Locations: []schemav1.InMemoryLocation{
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 1}, {FunctionId: 2}},
			},
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 0}},
			},
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 3}},
			},
			{},
		},
	}
	for _, tc := range []struct {
		path          []string
		total         int64
		mainSelf      int64
		functionTotal int64
	}{
		{nil, 112, 7, 105},
		{[]string{"main"}, 12, 7, 5},
		{[]string{"main", "F"}, 5, 0, 5},
		{[]string{"main", "F", "F"}, 5, 0, 5},
		{[]string{"main", "F", "F", "F"}, 0, 0, 0},
		{[]string{"F"}, 0, 0, 0},
		{[]string{"missing"}, 0, 0, 0},
	} {
		t.Run(strings.Join(tc.path, "/"), func(t *testing.T) {
			r := newFunctionTableSymbols(symbols, schemav1.Samples{
				Values: []uint64{5, 7, 100, 0},
			}, projectionSelector(tc.path))
			r.InsertStacktrace(0, []int32{0, 3, 1, 3})
			r.InsertStacktrace(1, []int32{1})
			r.InsertStacktrace(2, []int32{0, 2})
			r.InsertStacktrace(3, []int32{1})
			table := r.table()
			require.Equal(t, tc.total, table.Total)
			rows := make(map[string]*typesv1.FunctionStats)
			for _, f := range table.Functions {
				rows[f.Name] = f
			}
			if tc.total == 0 {
				require.Empty(t, rows)
				return
			}
			require.Equal(t, tc.mainSelf, rows["main"].Self)
			require.Equal(t, tc.functionTotal, rows["F"].Total)
			require.Equal(t, tc.functionTotal, rows["F"].Self)
			require.Equal(t, int64(len(rows)), table.TotalFunctions)
		})
	}
}

func TestResolverEmptyFunctions(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	r := NewResolver(context.Background(), s.db, WithResolverStackTraceSelector(projectionSelector([]string{"missing"})))
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	table, err := r.Functions()
	require.NoError(t, err)
	require.Empty(t, table.Functions)
	require.Zero(t, table.Total)
}

func TestResolverFunctionsCanceled(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	ctx, cancel := context.WithCancel(context.Background())
	r := NewResolver(ctx, s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	cancel()
	_, err := r.Functions()
	require.ErrorIs(t, err, context.Canceled)
}

func TestProjectionVisitorsSkipNonPositiveValues(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"F"},
		Functions: []schemav1.InMemoryFunction{{Name: 0}},
		Locations: []schemav1.InMemoryLocation{{
			Line: []schemav1.InMemoryLine{{FunctionId: 0}},
		}},
	}
	samples := schemav1.Samples{Values: []uint64{0, ^uint64(0), 5}}
	table := newFunctionTableSymbols(symbols, samples, nil)
	for i := range samples.Values {
		table.InsertStacktrace(uint32(i), []int32{0})
	}
	require.Equal(t, int64(5), table.table().Total)
	require.Equal(t, int64(5), table.table().Functions[0].Self)
}

func TestResolverFunctionsMatchUntruncatedProfile(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	tree, err := r.Tree()
	require.NoError(t, err)
	got, err := r.Functions()
	require.NoError(t, err)
	want := make(map[string]*typesv1.FunctionStats)
	var total int64
	tree.IterateStacks(func(_ model.FunctionName, value int64, stack []model.FunctionName) {
		if value <= 0 {
			return
		}
		total += value
		seen := make(map[string]bool)
		for i, name := range stack {
			f := want[string(name)]
			if f == nil {
				f = &typesv1.FunctionStats{Name: string(name)}
				want[string(name)] = f
			}
			if !seen[string(name)] {
				f.Total += value
				seen[string(name)] = true
			}
			if i == 0 {
				f.Self += value
			}
		}
	})
	require.Equal(t, total, got.Total)
	require.Equal(t, int64(len(want)), got.TotalFunctions)
	for _, f := range got.Functions {
		require.Equal(t, want[f.Name], f)
	}
	require.NotEmpty(t, got.Functions)
}

func BenchmarkResolverFunctions(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/big-profile.pb.gz"}})
	b.ReportAllocs()
	for b.Loop() {
		r := NewResolver(context.Background(), s.db)
		r.AddSamples(0, s.indexed[0][0].Samples)
		table, err := r.Functions()
		r.Release()
		require.NoError(b, err)
		require.NotEmpty(b, table.Functions)
	}
}

func projectionSelector(path []string) *typesv1.StackTraceSelector {
	selector := &typesv1.StackTraceSelector{}
	for _, name := range path {
		selector.CallSite = append(selector.CallSite, &typesv1.Location{Name: name})
	}
	return selector
}
