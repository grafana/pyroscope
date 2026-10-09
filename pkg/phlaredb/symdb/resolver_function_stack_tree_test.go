package symdb

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type projectionCase struct {
	path    []string
	options *typesv1.FunctionTreeOptions
}

func projectionCases(stacks [][]string, rng *rand.Rand, n int) []projectionCase {
	var cases []projectionCase
	depths := []*int32{nil, proto.Int32(0), proto.Int32(3)}
	add := func(path []string, direction typesv1.FunctionTreeDirection, selection typesv1.FunctionTreeSelection) {
		for _, depth := range depths {
			cases = append(cases, projectionCase{path: path, options: &typesv1.FunctionTreeOptions{
				Direction: direction,
				Selection: selection,
				MaxDepth:  depth,
			}})
		}
	}
	add(nil, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH)
	add([]string{"missing"}, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH)
	add([]string{"missing"}, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
	for range n {
		stack := stacks[rng.Intn(len(stacks))]
		if len(stack) == 0 {
			continue
		}
		prefix := stack[:1+rng.Intn(min(len(stack), 4))]
		add(prefix, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH)
		start := rng.Intn(len(stack))
		chain := stack[start : start+1+rng.Intn(min(len(stack)-start, 3))]
		add(chain, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
		add(chain, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
		add(chain[len(chain)-1:], typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
	}
	return cases
}

func functionStackIndex(t *testing.T, tree *model.FunctionStackTree) *model.FunctionStackIndex {
	t.Helper()
	b, err := proto.Marshal(tree.Proto())
	require.NoError(t, err)
	decoded := model.NewFunctionStackTree()
	encoded := new(queryv1.FunctionStackTree)
	require.NoError(t, proto.Unmarshal(b, encoded))
	require.NoError(t, decoded.MergeProto(encoded))
	x, err := model.NewFunctionStackIndex(decoded.Proto())
	require.NoError(t, err)
	return x
}

func treeStacks(t *testing.T, r *Resolver) [][]string {
	t.Helper()
	full, err := r.Tree()
	require.NoError(t, err)
	var stacks [][]string
	full.IterateStacks(func(_ model.FunctionName, _ int64, stack []model.FunctionName) {
		names := make([]string, len(stack))
		for i, name := range stack {
			names[i] = string(name)
		}
		slices.Reverse(names)
		stacks = append(stacks, names)
	})
	return stacks
}

func TestFunctionStackTreeMatchesResolverProjections(t *testing.T) {
	for name, newReader := range map[string]func(t *testing.T) (SymbolsReader, map[uint64][]schemav1.InMemoryProfile){
		"memory": func(t *testing.T) (SymbolsReader, map[uint64][]schemav1.InMemoryProfile) {
			s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}, {"testdata/profile.pb.gz"}})
			return s.db, s.indexed
		},
		"block": func(t *testing.T) (SymbolsReader, map[uint64][]schemav1.InMemoryProfile) {
			s := newBlockSuite(t, [][]string{{"testdata/profile.pb.gz"}, {"testdata/profile.pb.gz"}})
			t.Cleanup(s.teardown)
			return s.reader, s.indexed
		},
	} {
		t.Run(name, func(t *testing.T) {
			reader, indexed := newReader(t)
			resolver := func(opts ...ResolverOption) *Resolver {
				r := NewResolver(context.Background(), reader, opts...)
				for p, profiles := range indexed {
					r.AddSamples(p, profiles[0].Samples)
				}
				return r
			}
			r := resolver(WithResolverMaxNodes(0))
			stacks := treeStacks(t, r)
			tree, err := r.FunctionStackTree()
			r.Release()
			require.NoError(t, err)
			x := functionStackIndex(t, tree)

			rng := rand.New(rand.NewSource(1))
			for _, c := range projectionCases(stacks, rng, 12) {
				t.Run(fmt.Sprintf("%s/%s/%v/%q", c.options.Selection, c.options.Direction, c.options.MaxDepth, c.path), func(t *testing.T) {
					r := resolver(WithResolverStackTraceSelector(projectionSelector(c.path)))
					defer r.Release()
					want, err := r.FunctionTree(c.options)
					require.NoError(t, err)
					require.True(t, want.EqualVT(x.FunctionTree(c.path, c.options)))
					if c.options.GetSelection() == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH && c.options.MaxDepth == nil {
						wantTable, err := r.Functions()
						require.NoError(t, err)
						require.True(t, wantTable.EqualVT(x.Functions(c.path)))
					}
				})
			}
		})
	}
}

func TestStackTreeSymbolsMatchesProjectionSymbols(t *testing.T) {
	symbols := &Symbols{
		Strings:   []string{"main", "F", "F", "leaf", "", "G"},
		Functions: []schemav1.InMemoryFunction{{Name: 0}, {Name: 1}, {Name: 2}, {Name: 3}, {Name: 4}, {Name: 5}},
		Locations: []schemav1.InMemoryLocation{
			{Line: []schemav1.InMemoryLine{{FunctionId: 3}, {FunctionId: 2}}},
			{Line: []schemav1.InMemoryLine{{FunctionId: 1}}},
			{Line: []schemav1.InMemoryLine{{FunctionId: 0}}},
			{Line: []schemav1.InMemoryLine{{FunctionId: 4}, {FunctionId: 5}}},
			{},
		},
	}
	locations := [][]int32{{0, 1, 2}, {1, 2}, {0, 3}, {2}, {4, 0, 4, 1, 2, 4}, {4}, {}, {3, 1, 1, 2}, {0, 2}, {1}}
	samples := schemav1.Samples{Values: []uint64{5, 7, 11, 13, 3, 17, 19, 23, 0, ^uint64(0)}}
	insert := func(dst interface {
		InsertStacktrace(uint32, []int32)
	}) {
		for i, l := range locations {
			dst.InsertStacktrace(uint32(i), l)
		}
	}
	stacks := newStackTreeSymbols(symbols, samples)
	insert(stacks)
	tree := model.NewFunctionStackTree()
	tree.MergeStacktraceTree(stacks.tree, stacks.names)
	x := functionStackIndex(t, tree)

	paths := [][]string{nil, {"main"}, {"F"}, {"main", "F"}, {"F", "F"}, {"main", "F", "F"}, {""}, {"G", ""}, {"main", "F", "G"}, {"leaf"}, {"missing"}}
	for _, path := range paths {
		table := newFunctionTableSymbols(symbols, samples, projectionSelector(path))
		insert(table)
		m := model.NewFunctionTableMerger()
		m.Merge(table.table())
		require.True(t, m.Table().EqualVT(x.Functions(path)), "functions %q", path)
		for _, c := range projectionCases([][]string{path}, rand.New(rand.NewSource(0)), 0) {
			c.path = path
			for _, o := range []*typesv1.FunctionTreeOptions{
				{Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH, MaxDepth: c.options.MaxDepth},
				{Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, MaxDepth: c.options.MaxDepth},
				{Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, MaxDepth: c.options.MaxDepth},
				{Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, MaxDepth: c.options.MaxDepth},
			} {
				if o.Selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN &&
					(len(path) == 0 || (o.Direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH && len(path) != 1)) {
					continue
				}
				projection := newCallTreeSymbols(symbols, samples, projectionSelector(path), o)
				insert(projection)
				m := model.NewFunctionTreeMerger(o)
				if projection.valid {
					m.Merge(projection.builder.Tree())
				}
				want := m.Tree()
				got := x.FunctionTree(path, o)
				require.True(t, want.EqualVT(got), "%s %s %v %q\nwant %v\ngot  %v", o.Selection, o.Direction, o.MaxDepth, path, want, got)
			}
		}
	}
}
