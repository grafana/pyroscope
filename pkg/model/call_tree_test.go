package model

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func callTreeChild(t *testing.T, n *typesv1.CallTreeNode, name string) *typesv1.CallTreeNode {
	t.Helper()
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("child %q not found in %v", name, n)
	return nil
}

func TestCallTreeChildren(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 128} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			b := testCallTreeBuilder([]string{"main", "F"}, depth, false)
			b.Insert([]string{"main", "F"}, 3)
			b.Insert([]string{"main", "F", "X"}, 5)
			b.Insert([]string{"main", "F", "X", "deep"}, 7)
			b.Insert([]string{"main", "F", "other"}, 2)
			r := b.Tree().Root
			require.Equal(t, "F", r.Name)
			require.Equal(t, int64(17), r.Total)
			require.Equal(t, int64(3), r.Self)
			require.True(t, r.HasChildren)
			if depth == 0 {
				require.Empty(t, r.Children)
				return
			}
			require.Len(t, r.Children, 2)
			x := callTreeChild(t, r, "X")
			require.Equal(t, int64(12), x.Total)
			require.Equal(t, int64(5), x.Self)
			require.True(t, x.HasChildren)
			require.Equal(t, int64(2), callTreeChild(t, r, "other").Self)
			if depth == 1 {
				require.Empty(t, x.Children)
			} else {
				d := callTreeChild(t, x, "deep")
				require.Equal(t, int64(7), d.Self)
				require.False(t, d.HasChildren)
			}
		})
	}
}

func TestCallTreeRootReturnsEverySibling(t *testing.T) {
	b := testCallTreeBuilder(nil, 1, false)
	for i := 0; i < 1000; i++ {
		b.Insert([]string{fmt.Sprintf("f%04d", i), "hidden"}, 1)
	}
	r := b.Tree().Root
	require.Empty(t, r.Name)
	require.Len(t, r.Children, 1000)
	require.Equal(t, int64(1000), r.Total)
	for i, c := range r.Children {
		require.Equal(t, fmt.Sprintf("f%04d", i), c.Name)
		require.Equal(t, int64(1), c.Total)
		require.Zero(t, c.Self)
		require.True(t, c.HasChildren)
		require.Empty(t, c.Children)
	}
}

func TestCallTreeParents(t *testing.T) {
	stacks := []struct {
		stack []string
		value int64
	}{
		{[]string{"main", "A", "F", "X"}, 5},
		{[]string{"main", "B", "F"}, 3},
		{[]string{"top", "A", "F", "Z"}, 7},
		{[]string{"main", "A", "G", "F"}, 2},
		{[]string{"main", "F", "F"}, 1},
	}
	for _, tc := range []struct {
		path     []string
		total    int64
		self     int64
		children map[string]int64
	}{
		{[]string{"F"}, 19, 6, map[string]int64{"A": 12, "B": 3, "G": 2, "F": 1, "main": 1}},
		{[]string{"A", "F"}, 12, 0, map[string]int64{"main": 5, "top": 7}},
		{[]string{"A", "G", "F"}, 2, 0, map[string]int64{"main": 2}},
		{[]string{"F", "F"}, 1, 0, map[string]int64{"main": 1}},
		{[]string{"main", "A", "F"}, 5, 0, map[string]int64{}},
	} {
		t.Run(fmt.Sprint(tc.path), func(t *testing.T) {
			originalPath := slices.Clone(tc.path)
			b := testCallTreeBuilder(tc.path, 1, true)
			require.Equal(t, originalPath, tc.path, "building callers must not mutate the selector")
			for _, s := range stacks {
				b.Insert(s.stack, s.value)
			}
			r := b.Tree().Root
			require.Equal(t, tc.path[0], r.Name)
			require.Equal(t, tc.total, r.Total)
			require.Equal(t, tc.self, r.Self)
			require.Equal(t, len(tc.children) > 0, r.HasChildren)
			require.Len(t, r.Children, len(tc.children))
			for name, value := range tc.children {
				require.Equal(t, value, callTreeChild(t, r, name).Total)
			}
		})
	}
}

func TestCallTreeParentsDepthAndMissing(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 3} {
		b := testCallTreeBuilder([]string{"F"}, depth, true)
		b.Insert([]string{"root", "caller", "F", "leaf"}, 7)
		n := b.Tree().Root
		for i := 0; i < min(depth, 2); i++ {
			require.True(t, n.HasChildren)
			require.Len(t, n.Children, 1)
			n = n.Children[0]
			require.Equal(t, int64(7), n.Total)
			require.Zero(t, n.Self)
		}
		require.Empty(t, n.Children)
		require.Equal(t, depth < 2, n.HasChildren)
	}
	b := testCallTreeBuilder([]string{"missing"}, 2, true)
	b.Insert([]string{"root", "F"}, 1)
	require.Nil(t, b.Tree().Root)
}

func TestCallTreeMergeProjectionBeforeMerge(t *testing.T) {
	for _, parents := range []bool{false, true} {
		path := []string{"root"}
		if parents {
			path = []string{"F"}
		}
		all := testCallTreeBuilder(path, 2, parents)
		var m callTreeMerger
		for _, stack := range [][]string{{"root", "A", "F", "leaf"}, {"root", "B", "F"}, {"root", "A", "F"}} {
			part := testCallTreeBuilder(path, 2, parents)
			part.Insert(stack, 3)
			m.merge(part.Tree())
			all.Insert(stack, 3)
		}
		m.merge(nil)
		m.merge(&typesv1.CallTree{})
		require.Equal(t, all.Tree(), m.tree())
	}
}

func BenchmarkCallTreeProjection(b *testing.B) {
	stacks := make([][]string, 20000)
	fullStacks := make([][]FunctionName, len(stacks))
	for i := range stacks {
		stack := []string{"root", fmt.Sprintf("branch%d", i%100)}
		for depth := 0; depth < 32; depth++ {
			stack = append(stack, fmt.Sprintf("f%d_%d", i, depth))
		}
		stacks[i] = stack
		for _, name := range stack {
			fullStacks[i] = append(fullStacks[i], FunctionName(name))
		}
	}
	b.Run("children_depth_1", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tree := testCallTreeBuilder([]string{"root"}, 1, false)
			for _, stack := range stacks {
				tree.Insert(stack, 1)
			}
			_ = tree.Tree()
		}
	})
	b.Run("full_function_tree", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tree := new(FunctionNameTree)
			for _, stack := range fullStacks {
				tree.InsertStack(1, stack...)
			}
			_ = tree.Bytes(0, nil)
		}
	})
}

func testCallTreeBuilder(path []string, depth int, callers bool) *callTreeBuilder[string] {
	if callers {
		return newCallTreeBuilder(path, depth, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN, func(name string) string {
			return name
		})
	}
	return newCallTreeBuilder(path, depth, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH, func(name string) string {
		return name
	})
}
