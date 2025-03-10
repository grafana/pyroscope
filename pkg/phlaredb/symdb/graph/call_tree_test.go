package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var testTree1dfs = &CallTree{
	// A perfect tree for our purposes.
	//
	// The layout is very important because many operations rely on the locality
	// optimized for DFS. For example, accessing the parent node or the first
	// child is significantly faster if those are located close to each other:
	// ideally, in the same cache line.
	//
	// For example, in case of DFS layout, we can easily answer questions like
	// "whether node X is a descendant of node A" because we know that all the
	// descendants of A are located in the range (A, B), where B is the next
	// sibling of A.
	//
	// This is a very important use case: this way we can quickly
	// filter out leaves that are not part of a given call.
	//
	// | ┌────────────────────────┐
	// | │ 1                      │
	// | ├────┬─────────┬────┬────┤
	// | │ 2  │ 6       │ 10 │ 11 │
	// | ├────┼────┬────┼────┴────┘
	// | │ 3  │ 7  │ 9  │
	// | ├────┼────┼────┘
	// | │ 4  │ 8  │
	// | ├────┼────┘
	// | │ 5  │
	// | └────┘
	//
	// i   0    1    2    3    4    5    6    7    8    9    10   11
	// ─ ┌────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┐
	// v │ 0  │ 1  │ 2  │ 3  │ 4  │ 5  │ 6  │ 7  │ 8  │ 9  │ 10 │ 11 │
	// p │ -  │ 0  │ 1  │ 2  │ 3  │ 4  │ 1  │ 6  │ 7  │ 6  │ 1  │ 1  │
	// f │ 1  │ 2  │ 3  │ 4  │ 5  │ -  │ 7  │ 8  │ -  │ -  │ -  │ -  │
	// n │ -  │ -  │ 6  │ -  │ -  │ -  │ 10 │ 9  │ -  │ -  │ 11 │ -  │
	// w │ 0  │ 5  │ 1  │ 1  │ 1  │ 1  │ 2  │ 1  │ 1  │ 1  │ 1  │ 1  │
	// s │ 0  │ 0  │ 0  │ 0  │ 0  │ 1  │ 0  │ 0  │ 1  │ 1  │ 1  │ 1  │
	// ─ └────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴────┘
	// l   0    1    2    3    4    5    2    3    4    3    2    2   // Levels.
	// d   0    10   3    2    1    0    3    1    0    0    0    0   // Descendants.
	// D   0    5    5    5    5    5    4    4    4    3    2    2   // Depth.

	nodes: []node{
		// Node 0 (root)
		{v: 0, p: sentinel, f: 1, n: sentinel, w: 0, s: 0},
		{v: 1, p: 0, f: 2, n: sentinel, w: 5, s: 0},
		{v: 2, p: 1, f: 3, n: 6, w: 1, s: 0},
		{v: 3, p: 2, f: 4, n: sentinel, w: 1, s: 0},
		{v: 4, p: 3, f: 5, n: sentinel, w: 1, s: 0},
		{v: 5, p: 4, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 6, p: 1, f: 7, n: 10, w: 2, s: 0},
		{v: 7, p: 6, f: 8, n: 9, w: 1, s: 0},
		{v: 8, p: 7, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 9, p: 6, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 10, p: 1, f: sentinel, n: 11, w: 1, s: 1},
		{v: 11, p: 1, f: sentinel, n: sentinel, w: 1, s: 1},
	},
}

var testTree2dfs = &CallTree{
	// Same as testTree1dfs but with children listed in the reverse order.
	// This is happening, because of the LIFO stack we use in the merge
	// operation (DFS traversal): the last child is the first to be processed.
	// It can be "fixed" by reversing nodes on the stack, but it's not worth
	// it: this is a valid and fairly balanced layout for DFS traversal.
	//
	// However, it's not optimal. During the merge operation, which is done
	// in-place, and typically spans multiple trees, it's better to first
	// handle "cheaper" merges, and then fix the layout by transforming, to
	// ensure that the deepest branches always go first.
	//
	// | ┌────────────────────────┐
	// | │ 1                      │
	// | ├────┬────┬─────────┬────┤
	// | │ 11 │ 10 │ 6       │ 2  │
	// | └────┴────┼────┬────┼────┤
	// |           │ 9  │ 7  │ 3  │
	// |           └────┼────┼────┤
	// |                │ 8  │ 4  │
	// |                └────┼────┤
	// |                     │ 5  │
	// |                     └────┘
	//
	// i   0    1    2    3    4    5    6    7    8    9    10   11
	// ─ ┌────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┐
	// v │ 0  │ 1  │ 11 │ 10 │ 6  │ 9  │ 7  │ 8  │ 2  │ 3  │ 4  │ 5  │
	// p │ -  │ 0  │ 1  │ 1  │ 1  │ 4  │ 4  │ 6  │ 1  │ 8  │ 9  │ 10 │
	// f │ 1  │ 2  │ -  │ -  │ 5  │ -  │ 7  │ -  │ 9  │ 10 │ 11 │ -  │
	// n │ -  │ -  │ 3  │ 4  │ 8  │ 6  │ -  │ -  │ -  │ -  │ -  │ -  │
	// w │ 0  │ 5  │ 1  │ 1  │ 2  │ 1  │ 1  │ 1  │ 1  │ 1  │ 1  │ 1  │
	// s │ 0  │ 0  │ 1  │ 1  │ 0  │ 1  │ 0  │ 1  │ 0  │ 0  │ 0  │ 1  │
	// ─ └────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴────┘

	nodes: []node{
		{v: 0, p: sentinel, f: 1, n: sentinel, w: 0, s: 0},
		{v: 1, p: 0, f: 2, n: sentinel, w: 5, s: 0},
		{v: 11, p: 1, f: sentinel, n: 3, w: 1, s: 1},
		{v: 10, p: 1, f: sentinel, n: 4, w: 1, s: 1},
		{v: 6, p: 1, f: 5, n: 8, w: 2, s: 0},
		{v: 9, p: 4, f: sentinel, n: 6, w: 1, s: 1},
		{v: 7, p: 4, f: 7, n: sentinel, w: 1, s: 0},
		{v: 8, p: 6, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 2, p: 1, f: 9, n: sentinel, w: 1, s: 0},
		{v: 3, p: 8, f: 10, n: sentinel, w: 1, s: 0},
		{v: 4, p: 9, f: 11, n: sentinel, w: 1, s: 0},
		{v: 5, p: 10, f: sentinel, n: sentinel, w: 1, s: 1},
	},
}

var testTree3bfs = &CallTree{
	// BFS layout lacks a property of locality we need for processing,
	// such as trimming, LCA search, node search, subtree eviction and
	// so on.
	//
	// | ┌────────────────────────┐
	// | │ 1                      │
	// | ├────┬─────────┬────┬────┤
	// | │ 2  │ 6       │ 10 │ 11 │
	// | ├────┼────┬────┼────┴────┘
	// | │ 3  │ 7  │ 9  │
	// | ├────┼────┼────┘
	// | │ 4  │ 8  │
	// | ├────┼────┘
	// | │ 5  │
	// | └────┘
	//
	// i   0    1    2    3    4    5    6    7    8    9    10   11
	// ─ ┌────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┬────┐
	// v │ 0  │ 1  │ 2  │ 6  │ 10 │ 11 │ 3  │ 7  │ 9  │ 4  │ 8  │ 5  │
	// p │ -  │ 0  │ 1  │ 1  │ 1  │ 1  │ 2  │ 3  │ 3  │ 6  │ 7  │ 9  │
	// f │ 1  │ 2  │ 6  │ 7  │ -  │ -  │ 9  │ 10 │ -  │ -  │ -  │ -  │
	// n │ -  │ -  │ 3  │ 4  │ 5  │ -  │ -  │ 8  │ -  │ -  │ -  │ -  │
	// w │ 0  │ 5  │ 1  │ 2  │ 1  │ 1  │ 1  │ 1  │ 1  │ 1  │ 1  │ 1  │
	// s │ 0  │ 0  │ 0  │ 0  │ 1  │ 1  │ 0  │ 0  │ 1  │ 0  │ 1  │ 1  │
	// ─ └────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴────┘

	nodes: []node{
		{v: 0, p: sentinel, f: 1, n: sentinel, w: 0, s: 0},
		{v: 1, p: 0, f: 2, n: sentinel, w: 5, s: 0},
		{v: 2, p: 1, f: 6, n: 3, w: 1, s: 0},
		{v: 6, p: 1, f: 7, n: 4, w: 2, s: 0},
		{v: 10, p: 1, f: sentinel, n: 5, w: 1, s: 1},
		{v: 11, p: 1, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 3, p: 2, f: 9, n: sentinel, w: 1, s: 0},
		{v: 7, p: 3, f: 10, n: 8, w: 1, s: 0},
		{v: 9, p: 3, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 4, p: 6, f: 11, n: sentinel, w: 1, s: 0},
		{v: 8, p: 7, f: sentinel, n: sentinel, w: 1, s: 1},
		{v: 5, p: 9, f: sentinel, n: sentinel, w: 1, s: 1},
	},
}

func Test_levels(t *testing.T) {
	tree := testTree1dfs.Clone()
	c := make([]int32, len(tree.nodes))
	tree.levels(c)
	expected := []int32{0, 1, 2, 3, 4, 5, 2, 3, 4, 3, 2, 2}
	assert.Equal(t, expected, c)
}

func Test_descendants(t *testing.T) {
	tree := testTree1dfs.Clone()
	c := make([]int32, len(tree.nodes))
	tree.descendants(c)
	expected := []int32{0, 10, 3, 2, 1, 0, 3, 1, 0, 0, 0, 0}
	assert.Equal(t, expected, c)
}

func Test_depth(t *testing.T) {
	tree := testTree1dfs.Clone()
	c := make([]int32, len(tree.nodes))
	tree.depth(c)
	expected := []int32{0, 5, 5, 5, 5, 5, 4, 4, 4, 3, 2, 2}
	assert.Equal(t, expected, c)
}

func Test_merge_self(t *testing.T) {
	for _, test := range []struct {
		dst, src *CallTree
	}{
		{dst: testTree1dfs.Clone(), src: testTree1dfs.Clone()},
		{dst: testTree1dfs.Clone(), src: testTree2dfs.Clone()},
		{dst: testTree1dfs.Clone(), src: testTree3bfs.Clone()},

		{dst: testTree2dfs.Clone(), src: testTree1dfs.Clone()},
		{dst: testTree2dfs.Clone(), src: testTree2dfs.Clone()},
		{dst: testTree2dfs.Clone(), src: testTree3bfs.Clone()},

		{dst: testTree3bfs.Clone(), src: testTree1dfs.Clone()},
		{dst: testTree3bfs.Clone(), src: testTree2dfs.Clone()},
		{dst: testTree3bfs.Clone(), src: testTree3bfs.Clone()},
	} {
		// We expect the original structure to preserve.
		expected := test.dst.Clone()
		for i := range expected.nodes {
			expected.nodes[i].s *= 2
			expected.nodes[i].w *= 2
		}
		test.dst.Merge(test.src)
		assert.Equal(t, expected, test.dst)
	}
}

func Test_merge_empty(t *testing.T) {
	// Note that testTree1dfs transforms to testTree2dfs,
	// and vice-versa.
	src := testTree1dfs.Clone()
	dst := NewCallTree(len(src.nodes))
	dst.Merge(src)
	// The tree is empty, so the merge should be the
	// same as the source, but with inverse order of
	// children.
	assert.Equal(t, testTree2dfs, dst)

	src = testTree2dfs.Clone()
	dst = NewCallTree(len(src.nodes))
	dst.Merge(src)
	assert.Equal(t, testTree1dfs, dst)
}

func Test_merge_combine(t *testing.T) {
	// Combine all three trees of different layouts.
	dst := testTree1dfs.Clone()
	expected := dst.Clone()
	for i := range expected.nodes {
		expected.nodes[i].s *= 4
		expected.nodes[i].w *= 4
	}
	dst.Merge(testTree1dfs.Clone())
	dst.Merge(testTree2dfs.Clone())
	dst.Merge(testTree3bfs.Clone())
	assert.Equal(t, expected, dst)
}

func Test_transform(t *testing.T) {
	// Regardless of the order of the children, a tree
	// transform will always result in the same layout.
	src := testTree2dfs.Clone()
	dst := NewCallTree(len(src.nodes))
	src.transformDFS(dst, nil)
	assert.Equal(t, testTree1dfs, dst)
}

func Test_traverse(t *testing.T) {
	tree := testTree1dfs.Clone()
	t.Run("DFS", func(t *testing.T) {
		i := 0
		tree.traverseDFS(nil, func(n node) {
			assert.EqualValues(t, testTree1dfs.nodes[i].v, i)
			assert.EqualValues(t, testTree2dfs.nodes[i].v, n.v)
			i++
		})
	})

	t.Run("BFS", func(t *testing.T) {
		i := 0
		tree.traverseBFS(nil, func(n node) {
			assert.EqualValues(t, testTree3bfs.nodes[i].v, n.v)
			i++
		})
	})
}
