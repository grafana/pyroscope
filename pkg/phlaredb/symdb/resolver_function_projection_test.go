package symdb

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

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

func TestResolverEmptyFunctionProjections(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	for _, direction := range []typesv1.FunctionTreeDirection{typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH} {
		r := NewResolver(context.Background(), s.db, WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
			CallSite: []*typesv1.Location{{Name: "missing"}},
		}))
		r.AddSamples(0, s.indexed[0][0].Samples)
		table, err := r.Functions()
		require.NoError(t, err)
		require.Empty(t, table.Functions)
		require.Zero(t, table.Total)
		o := &typesv1.FunctionTreeOptions{
			Direction: direction,
			Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
			MaxDepth:  proto.Int32(4),
		}
		got, err := r.FunctionTree(o)
		r.Release()
		require.NoError(t, err)
		require.Equal(t, model.EmptyFunctionTree(o), got)
	}
}

func TestResolverCallTreeMatchesUntruncatedProfile(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	full := NewResolver(context.Background(), s.db, WithResolverMaxNodes(0))
	defer full.Release()
	full.AddSamples(0, s.indexed[0][0].Samples)
	tree, err := full.Tree()
	require.NoError(t, err)
	for _, parents := range []bool{false, true} {
		path := []string(nil)
		if parents {
			path = []string{"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie.(*Trie).Insert"}
		}
		selector := &typesv1.StackTraceSelector{}
		for _, name := range path {
			selector.CallSite = append(selector.CallSite, &typesv1.Location{Name: name})
		}
		r := NewResolver(context.Background(), s.db, WithResolverStackTraceSelector(selector), WithResolverMaxNodes(1))
		r.AddSamples(0, s.indexed[0][0].Samples)
		options := &typesv1.FunctionTreeOptions{
			Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
			Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
			MaxDepth:  proto.Int32(2),
		}
		if parents {
			options.Direction = typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS
			options.Selection = typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN
		}
		result, err := r.FunctionTree(options)
		got := result.Callees
		if parents {
			got = result.Callers
		}
		r.Release()
		require.NoError(t, err)
		want := model.NewFunctionTreeBuilder(path, options, func(name string) string {
			return name
		})
		tree.IterateStacks(func(_ model.FunctionName, value int64, stack []model.FunctionName) {
			names := make([]string, len(stack))
			for i, name := range stack {
				names[i] = string(name)
			}
			slices.Reverse(names)
			want.Insert(names, value)
		})
		require.NotNil(t, got.Root)
		require.Equal(t, want.Tree(), result, "projection must match untruncated samples")
	}
}

func TestResolverCallTreeCanceled(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	ctx, cancel := context.WithCancel(context.Background())
	r := NewResolver(ctx, s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	cancel()
	_, err := r.FunctionTree(&typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
		MaxDepth:  proto.Int32(1),
	})
	require.ErrorIs(t, err, context.Canceled)
	_, err = r.Functions()
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
	tree := newCallTreeSymbols(symbols, samples, nil, &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
	})
	for i := range samples.Values {
		table.InsertStacktrace(uint32(i), []int32{0})
		tree.InsertStacktrace(uint32(i), []int32{0})
	}
	require.Equal(t, int64(5), table.table().Total)
	require.Equal(t, int64(5), table.table().Functions[0].Self)
	require.Equal(t, int64(5), tree.builder.Tree().Callees.Root.Total)
	require.Equal(t, int64(5), tree.builder.Tree().Callees.Root.Children[0].Self)
}

func BenchmarkResolverCallTree(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/big-profile.pb.gz"}})
	r := NewResolver(context.Background(), s.db)
	r.AddSamples(0, s.indexed[0][0].Samples)
	full, err := r.Tree()
	r.Release()
	require.NoError(b, err)
	counts := map[string]int{}
	full.IterateStacks(func(_ model.FunctionName, _ int64, stack []model.FunctionName) {
		for _, name := range stack {
			counts[string(name)]++
		}
	})
	anchor, most := "", 0
	for name, count := range counts {
		if count > most || (count == most && name < anchor) {
			anchor, most = name, count
		}
	}
	b.Logf("PARENTS uses the most frequent function: %d occurrences in sampled stacks", most)
	for _, tc := range []struct {
		name    string
		depth   int
		parents bool
	}{
		{"children_1", 1, false}, {"children_4", 4, false}, {"parents_1", 1, true}, {"parents_4", 4, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var last *typesv1.FunctionTree
			for b.Loop() {
				opts := []ResolverOption{}
				if tc.parents {
					opts = append(opts, WithResolverStackTraceSelector(&typesv1.StackTraceSelector{
						CallSite: []*typesv1.Location{{Name: anchor}},
					}))
				}
				r := NewResolver(context.Background(), s.db, opts...)
				r.AddSamples(0, s.indexed[0][0].Samples)
				options := &typesv1.FunctionTreeOptions{
					Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
					Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
					MaxDepth:  proto.Int32(int32(tc.depth)),
				}
				if tc.parents {
					options.Direction = typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS
					options.Selection = typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN
				}
				last, err = r.FunctionTree(options)
				r.Release()
				require.NoError(b, err)
			}
			require.NotNil(b, last)
		})
	}
	for _, tc := range []struct {
		name     string
		maxNodes int64
	}{{"tree_16384", 16384}, {"tree_unlimited", 0}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r := NewResolver(context.Background(), s.db, WithResolverMaxNodes(tc.maxNodes))
				r.AddSamples(0, s.indexed[0][0].Samples)
				_, err := r.Tree()
				r.Release()
				require.NoError(b, err)
			}
		})
	}
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

func TestCallTreeSymbolsCanonicalFunctions(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"main", "F", "F", "leaf", "other"},
		Functions: []schemav1.InMemoryFunction{{Name: 0}, {Name: 1}, {Name: 2}, {Name: 3}, {Name: 4}},
		Locations: []schemav1.InMemoryLocation{
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 3}, {FunctionId: 2}},
			},
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 1}},
			},
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 0}},
			},
			{
				Line: []schemav1.InMemoryLine{{FunctionId: 4}},
			},
			{},
		},
	}
	locations := [][]int32{{0, 1, 2}, {1, 2}, {0, 3}, {2}, {4, 0, 4, 1, 2, 4}}
	stacks := [][]string{{"main", "F", "F", "leaf"}, {"main", "F"}, {"other", "F", "leaf"}, {"main"}, {"main", "F", "F", "leaf"}}
	samples := schemav1.Samples{
		Values: []uint64{5, 7, 11, 13, 3},
	}
	for _, selection := range []typesv1.FunctionTreeSelection{typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN} {
		for _, path := range [][]string{nil, {"main"}, {"F"}, {"main", "F"}, {"F", "F"}, {"F", "main"}, {"main", "F", "F", "F"}, {"missing"}} {
			for _, direction := range []typesv1.FunctionTreeDirection{typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH} {
				if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH && direction != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES {
					continue
				}
				if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN && (len(path) == 0 || (direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH && len(path) != 1)) {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%v", selection, direction, path), func(t *testing.T) {
					options := &typesv1.FunctionTreeOptions{
						Selection: selection,
						Direction: direction,
						MaxDepth:  proto.Int32(4),
					}
					projection := newCallTreeSymbols(symbols, samples, projectionSelector(path), options)
					require.Equal(t, projection.functionIndex[1], projection.functionIndex[2], "identical names with different string and function IDs must coalesce")
					want := model.NewFunctionTreeBuilder(path, options, func(name string) string {
						return name
					})
					for i, stack := range stacks {
						projection.InsertStacktrace(uint32(i), locations[i])
						if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH && (len(stack) < len(path) || !slices.Equal(stack[:len(path)], path)) {
							continue
						}
						want.Insert(stack, int64(samples.Values[i]))
					}
					got := projection.builder.Tree()
					require.True(t, want.Tree().EqualVT(got))
					if direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS {
						if slices.Equal(path, []string{"main", "F"}) {
							require.Equal(t, "main", got.Callers.Root.Name)
							require.Equal(t, int64(15), got.Callers.Root.Total)
							require.Empty(t, got.Callers.Root.Children)
						}
						if slices.Equal(path, []string{"F", "main"}) {
							require.Nil(t, got.Callers.Root)
						}
					}
					if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN && slices.Equal(path, []string{"F"}) {
						for _, tree := range []*typesv1.CallTree{got.Callers, got.Callees} {
							if tree != nil {
								require.Equal(t, int64(34), tree.Root.Total)
								require.Equal(t, int64(7), tree.Root.Self)
							}
						}
					}
				})
			}
		}
	}
}

func TestCallTreeSymbolsEmptyStack(t *testing.T) {
	projection := newCallTreeSymbols(&Symbols{}, schemav1.Samples{
		Values: []uint64{3},
	}, nil, &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
	})
	projection.InsertStacktrace(0, nil)
	root := projection.builder.Tree().Callees.Root
	require.Empty(t, root.Name)
	require.Equal(t, int64(3), root.Total)
	require.Equal(t, int64(3), root.Self)
	require.Empty(t, root.Children)
}
