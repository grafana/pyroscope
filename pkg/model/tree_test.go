package model

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Tree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stacks   []stacktraces
		expected func() *Tree
	}{
		{
			"empty",
			[]stacktraces{},
			func() *Tree { return &Tree{} },
		},
		{
			"double node single stack",
			[]stacktraces{
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
			},
			func() *Tree {
				tr := emptyTree()
				tr.add("bar", 0, 2).add("buz", 2, 2)
				return tr
			},
		},
		{
			"double node double stack",
			[]stacktraces{
				{
					locations: []string{"blip", "buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"blap", "blop", "buz", "bar"},
					value:     2,
				},
			},
			func() *Tree {
				tr := emptyTree()
				buz := tr.add("bar", 0, 3).add("buz", 0, 3)
				buz.add("blip", 1, 1)
				buz.add("blop", 0, 2).add("blap", 2, 2)
				return tr
			},
		},
		{
			"multiple stacks and duplicates nodes",
			[]stacktraces{
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"buz"},
					value:     1,
				},
				{
					locations: []string{"foo", "buz", "bar"},
					value:     1,
				},
				{
					locations: []string{"blop", "buz", "bar"},
					value:     2,
				},
				{
					locations: []string{"blip", "bar"},
					value:     4,
				},
			},
			func() *Tree {
				tr := emptyTree()

				bar := tr.add("bar", 0, 9)
				bar.add("blip", 4, 4)

				buz := bar.add("buz", 2, 5)
				buz.add("blop", 2, 2)
				buz.add("foo", 1, 1)

				tr.add("buz", 1, 1)
				return tr
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected := tc.expected().String()
			tr := newTree(tc.stacks).String()
			require.Equal(t, tr, expected, "tree should be equal got:%s\n expected:%s\n", tr, expected)
		})
	}
}

func Test_TreeMerge(t *testing.T) {
	type testCase struct {
		description        string
		src, dst, expected *Tree
	}

	testCases := func() []testCase {
		return []testCase{
			{
				description: "empty src",
				dst: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
				src: new(Tree),
				expected: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
			},
			{
				description: "empty dst",
				dst:         new(Tree),
				src: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
				expected: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
			},
			{
				description: "empty both",
				dst:         new(Tree),
				src:         new(Tree),
				expected:    new(Tree),
			},
			{
				description: "missing nodes in dst",
				dst: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
				src: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
					{locations: []string{"c", "b", "a1"}, value: 1},
					{locations: []string{"c", "b1", "a"}, value: 1},
					{locations: []string{"c1", "b", "a"}, value: 1},
				}),
				expected: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 2},
					{locations: []string{"c", "b", "a1"}, value: 1},
					{locations: []string{"c", "b1", "a"}, value: 1},
					{locations: []string{"c1", "b", "a"}, value: 1},
				}),
			},
			{
				description: "missing nodes in src",
				dst: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
					{locations: []string{"c", "b", "a1"}, value: 1},
					{locations: []string{"c", "b1", "a"}, value: 1},
					{locations: []string{"c1", "b", "a"}, value: 1},
				}),
				src: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
				expected: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 2},
					{locations: []string{"c", "b", "a1"}, value: 1},
					{locations: []string{"c", "b1", "a"}, value: 1},
					{locations: []string{"c1", "b", "a"}, value: 1},
				}),
			},
		}
	}

	t.Run("Tree.Merge", func(t *testing.T) {
		for _, tc := range testCases() {
			tc := tc
			t.Run(tc.description, func(t *testing.T) {
				tc.dst.Merge(tc.src)
				require.Equal(t, tc.expected.String(), tc.dst.String())
			})
		}
	})
}

func Test_Tree_MarshalUnmarshal(t *testing.T) {
	t.Run("empty tree", func(t *testing.T) {
		expected := new(Tree)
		var buf bytes.Buffer
		require.NoError(t, expected.MarshalTruncate(&buf, -1))
		actual, err := UnmarshalTree(buf.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected.String(), actual.String())
	})

	t.Run("non-empty tree", func(t *testing.T) {
		expected := newTree([]stacktraces{
			{locations: []string{"c", "b", "a"}, value: 1},
			{locations: []string{"c", "b", "a"}, value: 1},
			{locations: []string{"c1", "b", "a"}, value: 1},
			{locations: []string{"c", "b1", "a"}, value: 1},
			{locations: []string{"c1", "b1", "a"}, value: 1},
			{locations: []string{"c", "b", "a1"}, value: 1},
			{locations: []string{"c1", "b", "a1"}, value: 1},
			{locations: []string{"c", "b1", "a1"}, value: 1},
			{locations: []string{"c1", "b1", "a1"}, value: 1},
		})

		var buf bytes.Buffer
		require.NoError(t, expected.MarshalTruncate(&buf, -1))
		actual, err := UnmarshalTree(buf.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected.String(), actual.String())
	})

	t.Run("truncation", func(t *testing.T) {
		fullTree := newTree([]stacktraces{
			{locations: []string{"c", "b", "a"}, value: 1},
			{locations: []string{"c", "b", "a"}, value: 1},
			{locations: []string{"c1", "b", "a"}, value: 1},
			{locations: []string{"c", "b1", "a"}, value: 1},
			{locations: []string{"c1", "b1", "a"}, value: 1},
			{locations: []string{"c", "b", "a1"}, value: 1},
			{locations: []string{"c1", "b", "a1"}, value: 1},
			{locations: []string{"c", "b1", "a1"}, value: 1},
			{locations: []string{"c1", "b1", "a1"}, value: 1},
		})

		var buf bytes.Buffer
		require.NoError(t, fullTree.MarshalTruncate(&buf, 3))

		actual, err := UnmarshalTree(buf.Bytes())
		require.NoError(t, err)

		expected := newTree([]stacktraces{
			{locations: []string{"other", "b", "a"}, value: 3},
			{locations: []string{"other", "a"}, value: 2},
			{locations: []string{"other", "a1"}, value: 4},
		})

		require.Equal(t, expected.String(), actual.String())
	})
}

func emptyTree() *Tree {
	return &Tree{}
}

func newTree(stacks []stacktraces) *Tree {
	t := emptyTree()
	for _, stack := range stacks {
		if stack.value == 0 {
			continue
		}
		if t == nil {
			t = stackToTree(stack)
			continue
		}
		t.Merge(stackToTree(stack))
	}
	return t
}

type stacktraces struct {
	locations []string
	value     int64
}

func (t *Tree) add(name string, self, total int64) *node {
	new := &node{
		name:  name,
		self:  self,
		total: total,
	}
	t.root = append(t.root, new)
	return new
}

func (n *node) add(name string, self, total int64) *node {
	new := &node{
		parent: n,
		name:   name,
		self:   self,
		total:  total,
	}
	n.children = append(n.children, new)
	return new
}

func stackToTree(stack stacktraces) *Tree {
	t := emptyTree()
	if len(stack.locations) == 0 {
		return t
	}
	current := &node{
		self:  stack.value,
		total: stack.value,
		name:  stack.locations[0],
	}
	if len(stack.locations) == 1 {
		t.root = append(t.root, current)
		return t
	}
	remaining := stack.locations[1:]
	for len(remaining) > 0 {

		location := remaining[0]
		name := location
		remaining = remaining[1:]

		// This pack node with the same name as the next location
		// Disable for now but we might want to introduce it if we find it useful.
		// for len(remaining) != 0 {
		// 	if remaining[0].function == name {
		// 		remaining = remaining[1:]
		// 		continue
		// 	}
		// 	break
		// }

		parent := &node{
			children: []*node{current},
			total:    current.total,
			name:     name,
		}
		current.parent = parent
		current = parent
	}
	t.root = []*node{current}
	return t
}
