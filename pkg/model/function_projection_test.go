package model

import (
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestFunctionTreeSelectionAndRefinement(t *testing.T) {
	stacks := [][]string{{"main", "A", "F", "X", "leaf"}, {"top", "A", "F", "X"}, {"main", "B", "F"}, {"main", "X", "unrelated"}, {"main", "A", "unrelated"}}
	values := []int64{5, 7, 3, 100, 200}
	query := func(path []string, selection typesv1.FunctionTreeSelection, direction typesv1.FunctionTreeDirection, depth int32) *typesv1.FunctionTree {
		b := NewFunctionTreeBuilder(path, &typesv1.FunctionTreeOptions{
			Selection: selection,
			Direction: direction,
			MaxDepth:  proto.Int32(depth),
		}, func(name string) string {
			return name
		})
		for i, stack := range stacks {
			if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH && (len(stack) < len(path) || !slices.Equal(stack[:len(path)], path)) {
				continue
			}
			b.Insert(stack, values[i])
		}
		return b.Tree()
	}
	global := typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN
	callees := typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES
	callers := typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS
	both := query([]string{"F"}, global, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, 1)
	require.Equal(t, int64(15), both.Callees.Root.Total)
	require.Equal(t, int64(3), both.Callees.Root.Self)
	require.Equal(t, int64(15), both.Callers.Root.Total)
	require.Equal(t, int64(3), both.Callers.Root.Self)
	x := callTreeChild(t, both.Callees.Root, "X")
	require.True(t, x.HasChildren)
	require.Equal(t, int64(7), x.Self)
	require.Empty(t, x.Children)
	refined := query([]string{"F", "X"}, global, callees, 1)
	require.Equal(t, int64(12), refined.Callees.Root.Total)
	require.Equal(t, int64(5), callTreeChild(t, refined.Callees.Root, "leaf").Total)
	parents := query([]string{"A", "F"}, global, callers, 1)
	require.Equal(t, "A", parents.Callers.Root.Name)
	require.Equal(t, int64(12), parents.Callers.Root.Total)
	require.Equal(t, int64(5), callTreeChild(t, parents.Callers.Root, "main").Total)
	require.Equal(t, int64(7), callTreeChild(t, parents.Callers.Root, "top").Total)
	children := query([]string{"A", "F"}, global, callees, 1)
	require.Equal(t, "F", children.Callees.Root.Name)
	require.Equal(t, parents.Callers.Root.Total, children.Callees.Root.Total)
	require.Nil(t, query([]string{"F", "A"}, global, callers, 1).Callers.Root)
	rootPath := query([]string{"main", "A", "F"}, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH, callees, 0)
	require.Equal(t, int64(5), rootPath.Callees.Root.Total)
	require.Nil(t, rootPath.Callers)
	require.True(t, rootPath.Callees.Root.HasChildren)
	require.Empty(t, rootPath.Callees.Root.Children)
	require.Equal(t, both.Callers, query([]string{"F"}, global, callers, 1).Callers)
	require.Equal(t, both.Callees, query([]string{"F"}, global, callees, 1).Callees)
}

func TestFunctionProjectionRecursionAndMerge(t *testing.T) {
	options := &typesv1.FunctionTreeOptions{
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH,
	}
	trees := NewFunctionTreeMerger(options)
	tables := NewFunctionTableMerger()
	for _, value := range []int64{2, 3} {
		b := NewFunctionTreeBuilder([]string{"F"}, options, func(name string) string {
			return name
		})
		b.Insert([]string{"main", "F", "F"}, value)
		trees.Merge(b.Tree())
		tables.Merge(&typesv1.FunctionTable{
			Total: value,
			Functions: []*typesv1.FunctionStats{{
				Name:  "F",
				Self:  value,
				Total: value,
			}, {
				Name:  "main",
				Total: value,
			}},
		})
	}
	tree := trees.Tree()
	require.Equal(t, int64(10), tree.Callers.Root.Total)
	require.Equal(t, int64(10), tree.Callees.Root.Total)
	require.Equal(t, int64(5), tree.Callers.Root.Self)
	require.Equal(t, int64(5), tree.Callees.Root.Self)
	table := tables.Table()
	require.Equal(t, int64(5), table.Total)
	require.Equal(t, int64(2), table.TotalFunctions)
	require.Equal(t, "F", table.Functions[0].Name)
	require.Equal(t, int64(5), table.Functions[0].Total)
	require.Equal(t, int64(5), table.Functions[0].Self)
	// Caller terminal nodes represent the end of a stack, not the caller's self.
	require.Zero(t, callTreeChild(t, tree.Callers.Root, "main").Self)
}

func TestFunctionTableMergeDeterministicOrder(t *testing.T) {
	merger := NewFunctionTableMerger()
	part := &typesv1.FunctionTable{
		Total: 10,
		Functions: []*typesv1.FunctionStats{{
			Name:  "B",
			Total: 5,
			Self:  5,
		}, {
			Name:  "A",
			Total: 5,
			Self:  5,
		}, {
			Name:  "main",
			Total: 10,
		}},
	}
	merger.Merge(part)
	r := merger.Table()
	require.Equal(t, int64(10), r.Total)
	require.Equal(t, int64(3), r.TotalFunctions)
	require.Equal(t, "A", r.Functions[0].Name)
	require.Equal(t, "B", r.Functions[1].Name)
	require.Equal(t, "main", r.Functions[2].Name)
	merger.Merge(part)
	require.Equal(t, int64(5), r.Functions[0].Self, "built result must remain immutable after subsequent merges")
	require.Equal(t, int64(20), merger.Table().Total)
}

func TestFunctionTreeDefaultDepth(t *testing.T) {
	for _, tc := range []struct {
		name     string
		depth    *int32
		children int
	}{
		{"omitted", nil, 1},
		{"root only", proto.Int32(0), 0},
		{"one level", proto.Int32(1), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := &typesv1.FunctionTreeOptions{
				Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
				Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
				MaxDepth:  tc.depth,
			}
			builder := NewFunctionTreeBuilder([]string(nil), options, func(name string) string {
				return name
			})
			builder.Insert([]string{"main", "leaf"}, 3)
			root := builder.Tree().Callees.Root
			require.Equal(t, int64(3), root.Total)
			require.Len(t, root.Children, tc.children)
			require.True(t, root.HasChildren)
			if tc.children > 0 {
				require.True(t, root.Children[0].HasChildren)
				require.Empty(t, root.Children[0].Children)
			}
			require.Equal(t, tc.depth, options.MaxDepth)
		})
	}
}

func TestFunctionTreeResolvesNamesAtOutput(t *testing.T) {
	names := []string{"", "main", "F", "leaf"}
	lookups := 0
	builder := NewFunctionTreeBuilder([]int32{2}, &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
	}, func(id int32) string {
		lookups++
		return names[id]
	})
	for range 100 {
		builder.Insert([]int32{1, 2, 3}, 7)
	}
	require.Zero(t, lookups)
	tree := builder.Tree()
	require.Equal(t, 4, lookups)
	require.Equal(t, "F", tree.Callers.Root.Name)
	require.Equal(t, "main", tree.Callers.Root.Children[0].Name)
	require.Equal(t, "leaf", tree.Callees.Root.Children[0].Name)
	require.Equal(t, int64(700), tree.Callees.Root.Total)
}

func TestFunctionTreeMergerConcurrentSnapshots(t *testing.T) {
	const workers, iterations = 8, 100
	branch := &typesv1.CallTree{
		Root: &typesv1.CallTreeNode{
			Name:        "F",
			Total:       2,
			Self:        1,
			HasChildren: true,
			Children: []*typesv1.CallTreeNode{{
				Name:  "child",
				Total: 1,
				Self:  1,
			}},
		},
	}
	input := &typesv1.FunctionTree{
		Callers: branch,
		Callees: branch,
	}
	merger := NewFunctionTreeMerger(nil)
	var writers sync.WaitGroup
	start := make(chan struct{})
	for range workers {
		writers.Go(func() {
			<-start
			for range iterations {
				merger.Merge(input)
			}
		})
	}
	close(start)
	for range iterations {
		snapshot := merger.Tree()
		require.Equal(t, snapshot.Callers, snapshot.Callees, "both directions must reflect the same completed merges")
	}
	writers.Wait()
	snapshot := merger.Tree()
	require.Equal(t, int64(2*workers*iterations), snapshot.Callers.Root.Total)
	require.Equal(t, snapshot.Callers, snapshot.Callees)
	snapshot.Callers.Root.Children[0].Total = -1
	require.Equal(t, int64(workers*iterations), merger.Tree().Callers.Root.Children[0].Total)
	require.Equal(t, int64(1), branch.Root.Children[0].Total)
}

func TestEmptyFunctionTree(t *testing.T) {
	for _, direction := range []typesv1.FunctionTreeDirection{typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH} {
		o := &typesv1.FunctionTreeOptions{
			Direction: direction,
			Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
			MaxDepth:  proto.Int32(1),
		}
		b := NewFunctionTreeBuilder([]string{"missing"}, o, func(name string) string {
			return name
		})
		require.Equal(t, b.Tree(), EmptyFunctionTree(o))
		require.Equal(t, b.Tree(), NewFunctionTreeMerger(o).Tree())
	}
}
