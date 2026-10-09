package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStacktraceTreeEdgeIndex(t *testing.T) {
	// Multiples of 1024 collide at every table size used here. Include zero
	// and the truncation location (-1), and reuse locations under two parents.
	var batch [][]int32
	for _, parent := range []int32{7, 11} {
		batch = append(batch, []int32{-1, parent})
		for i := int32(0); i < 128; i++ {
			batch = append(batch, []int32{i * 1024, parent})
		}
		batch = append(batch, []int32{-1, 1024, parent})
	}
	tree := NewStacktraceTree(1)
	for pass := 0; pass < 2; pass++ {
		for _, stack := range batch {
			leaf := tree.Insert(stack, 1)
			require.Equal(t, stack[0], tree.Nodes[leaf].Location)
		}
		requireStacktraceInsertReference(t, tree, batch)
		for _, stack := range batch {
			tree.Insert(stack, 1)
		}
		requireStacktraceInsertReference(t, tree, append(append([][]int32(nil), batch...), batch...))
		require.Positive(t, tree.edges.count)
		require.Greater(t, len(tree.edges.slots), 16)
		for _, child := range tree.edges.slots {
			if child != 0 {
				n := tree.Nodes[child]
				indexed, _ := tree.edges.lookup(tree.Nodes, n.Parent, n.Location)
				require.Equal(t, child, indexed)
			}
		}
		capacity := cap(tree.edges.slots)
		tree.Reset()
		require.Zero(t, tree.edges.count)
		require.Equal(t, capacity, cap(tree.edges.slots))
		for _, slot := range tree.edges.slots {
			require.Zero(t, slot)
		}
		// Insert unrelated edges before another reset to detect stale entries
		// whose node indices may be reused for different locations.
		other := [][]int32{{99, 88}, {77, 88}, {66, 88}, {55, 88}, {44, 88}}
		insertStacktraceBatch(tree, other)
		requireStacktraceInsertReference(t, tree, other)
		tree.Reset()
	}
}

func TestStacktraceTreeEdgeIndexNarrowAndEmpty(t *testing.T) {
	tree := NewStacktraceTree(0)
	require.Zero(t, tree.Insert(nil, 3))
	require.Equal(t, int64(3), tree.Nodes[0].Value)
	tree.Insert([]int32{1, 2, 3}, 1)
	tree.Insert([]int32{1, 2, 3}, 1)
	require.Nil(t, tree.edges.slots, "single-child paths must not allocate an index")
	tree.Reset()
	requireStacktraceInsertReference(t, tree, nil)
	for i := int32(0); i < stacktraceUnindexedChildren; i++ {
		tree.Insert([]int32{i}, 1)
	}
	require.Nil(t, tree.edges.slots, "prefix children must not allocate an index")
	tree.Insert([]int32{stacktraceUnindexedChildren}, 1)
	require.Equal(t, 1, tree.edges.count)
}
