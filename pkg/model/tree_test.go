package model

import (
	"bytes"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

func Test_Tree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stacks   []stacktraces
		expected func() *FunctionNameTree
	}{
		{
			"empty",
			[]stacktraces{},
			func() *FunctionNameTree { return &FunctionNameTree{} },
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
			func() *FunctionNameTree {
				tr := emptyTree()
				bar := addNodeToTree(tr, "bar", 0, 2)
				addNodeToNode(bar, "buz", 2, 2)
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
			func() *FunctionNameTree {
				tr := emptyTree()
				bar := addNodeToTree(tr, "bar", 0, 3)
				buz := addNodeToNode(bar, "buz", 0, 3)
				addNodeToNode(buz, "blip", 1, 1)
				blop := addNodeToNode(buz, "blop", 0, 2)
				addNodeToNode(blop, "blap", 2, 2)
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
			func() *FunctionNameTree {
				tr := emptyTree()

				bar := addNodeToTree(tr, "bar", 0, 9)
				addNodeToNode(bar, "blip", 4, 4)

				buz := addNodeToNode(bar, "buz", 2, 5)
				addNodeToNode(buz, "blop", 2, 2)
				addNodeToNode(buz, "foo", 1, 1)

				addNodeToTree(tr, "buz", 1, 1)
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
		src, dst, expected *FunctionNameTree
	}

	testCases := func() []testCase {
		return []testCase{
			{
				description: "empty src",
				dst: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
				src: new(FunctionNameTree),
				expected: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
			},
			{
				description: "empty dst",
				dst:         new(FunctionNameTree),
				src: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
				expected: newTree([]stacktraces{
					{locations: []string{"c", "b", "a"}, value: 1},
				}),
			},
			{
				description: "empty both",
				dst:         new(FunctionNameTree),
				src:         new(FunctionNameTree),
				expected:    new(FunctionNameTree),
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

func Test_Tree_minValue(t *testing.T) {
	x := newTree([]stacktraces{
		{locations: []string{"c", "b", "a"}, value: 1},
		{locations: []string{"c", "b", "a"}, value: 1},
		{locations: []string{"c1", "b", "a"}, value: 1},
		{locations: []string{"c", "b1", "a"}, value: 1},
	})

	type testCase struct {
		desc     string
		maxNodes int64
		expected int64
	}

	testCases := []*testCase{
		{desc: "tree greater than max nodes", maxNodes: 2, expected: 3},
		{desc: "tree less than max nodes", maxNodes: math.MaxInt64, expected: 0},
		{desc: "zero max nodes", maxNodes: 0, expected: 0},
		{desc: "negative max nodes", maxNodes: -1, expected: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			assert.Equal(t, tc.expected, x.minValue(tc.maxNodes))
		})
	}
}

func Test_Tree_MarshalUnmarshal(t *testing.T) {
	t.Run("empty tree", func(t *testing.T) {
		expected := new(FunctionNameTree)
		var buf bytes.Buffer
		require.NoError(t, expected.MarshalTruncate(&buf, -1, nil))
		actual, err := UnmarshalTree[FuntionName, FuntionNameI](buf.Bytes())
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
		require.NoError(t, expected.MarshalTruncate(&buf, -1, nil))
		actual, err := UnmarshalTree[FuntionName, FuntionNameI](buf.Bytes())
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
		require.NoError(t, fullTree.MarshalTruncate(&buf, 3, nil))

		actual, err := UnmarshalTree[FuntionName, FuntionNameI](buf.Bytes())
		require.NoError(t, err)

		expected := newTree([]stacktraces{
			{locations: []string{"other", "b", "a"}, value: 3},
			{locations: []string{"other", "a"}, value: 2},
			{locations: []string{"other", "a1"}, value: 4},
		})

		require.Equal(t, expected.String(), actual.String())
	})
}

func Test_FormatNames(t *testing.T) {
	x := newTree([]stacktraces{
		{locations: []string{"c0", "b0", "a0"}, value: 3},
		{locations: []string{"c1", "b0", "a0"}, value: 3},
		{locations: []string{"d0", "b1", "a0"}, value: 2},
		{locations: []string{"e1", "c1", "a1"}, value: 4},
		{locations: []string{"e2", "c1", "a2"}, value: 4},
	})
	x.FormatNodeNames(func(n FuntionName) FuntionName {
		s := string(n)
		if len(s) > 0 {
			s = s[:1]
			n = FuntionName(s)
		}
		return n
	})
	expected := newTree([]stacktraces{
		{locations: []string{"c", "b", "a"}, value: 3},
		{locations: []string{"c", "b", "a"}, value: 3},
		{locations: []string{"d", "b", "a"}, value: 2},
		{locations: []string{"e", "c", "a"}, value: 4},
		{locations: []string{"e", "c", "a"}, value: 4},
	})
	require.Equal(t, expected.String(), x.String())
}

func emptyTree() *FunctionNameTree {
	return &FunctionNameTree{}
}

func newTree(stacks []stacktraces) *FunctionNameTree {
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

func addNodeToTree(t *FunctionNameTree, name string, self, total int64) *node[FuntionName] {
	new := &node[FuntionName]{
		name:  FuntionName(name),
		self:  self,
		total: total,
	}
	t.root = append(t.root, new)
	return new
}

func addNodeToNode(n *node[FuntionName], name string, self, total int64) *node[FuntionName] {
	new := &node[FuntionName]{
		parent: n,
		name:   FuntionName(name),
		self:   self,
		total:  total,
	}
	n.children = append(n.children, new)
	return new
}

func stackToTree(stack stacktraces) *FunctionNameTree {
	t := emptyTree()
	if len(stack.locations) == 0 {
		return t
	}
	current := &node[FuntionName]{
		self:  stack.value,
		total: stack.value,
		name:  FuntionName(stack.locations[0]),
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

		parent := &node[FuntionName]{
			children: []*node[FuntionName]{current},
			total:    current.total,
			name:     FuntionName(name),
		}
		current.parent = parent
		current = parent
	}
	t.root = []*node[FuntionName]{current}
	return t
}

func Test_IterateStacks_LargeRootCount(t *testing.T) {
	// Test for bug fix: when tree has more than 1024 root nodes,
	// the initial capacity should be adjusted to avoid reallocation
	tree := emptyTree()

	// Create a tree with 1500 root nodes (more than default capacity of 1024)
	rootCount := 1500
	for i := 0; i < rootCount; i++ {
		tree.InsertStack(1, FuntionName("root"+strconv.Itoa(i)))
	}

	require.Equal(t, rootCount, len(tree.root), "should have %d root nodes", rootCount)

	// IterateStacks should handle this without issues
	visitedCount := 0
	tree.IterateStacks(func(name FuntionName, self int64, stack []FuntionName) {
		visitedCount++
		require.Equal(t, int64(1), self, "each node should have self=1")
		require.Equal(t, 1, len(stack), "each stack should have length 1")
	})

	require.Equal(t, rootCount, visitedCount, "should visit all %d root nodes", rootCount)
}

func Test_TreeFromBackendProfileSampleType(t *testing.T) {
	profile := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
			{Type: 3, Unit: 4},
		},
		StringTable: []string{
			"",
			"samples",
			"count",
			"cpu",
			"nanoseconds",
			"main",
			"foo",
			"bar",
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2},
				Value:      []int64{10, 20},
			},
			{
				LocationId: []uint64{2, 3},
				Value:      []int64{30, 60},
			},
		},
		Location: []*profilev1.Location{
			{Id: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2}}},
			{Id: 3, Line: []*profilev1.Line{{FunctionId: 3}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 5},
			{Id: 2, Name: 6},
			{Id: 3, Name: 7},
		},
	}

	t.Run("using first sample type (index 0)", func(t *testing.T) {
		treeBytes, err := TreeFromBackendProfileSampleType(profile, -1, 0)
		require.NoError(t, err)
		tree := MustUnmarshalTree[FuntionName, FuntionNameI](treeBytes)
		assert.Equal(t, int64(40), tree.Total())
	})

	t.Run("using second sample type (index 1)", func(t *testing.T) {
		treeBytes, err := TreeFromBackendProfileSampleType(profile, -1, 1)
		require.NoError(t, err)
		tree := MustUnmarshalTree[FuntionName, FuntionNameI](treeBytes)
		assert.Equal(t, int64(80), tree.Total())
	})

	t.Run("validates sample type index bounds", func(t *testing.T) {
		_, err := TreeFromBackendProfileSampleType(profile, -1, 99)
		require.Error(t, err)
	})

	t.Run("handles profile with no samples", func(t *testing.T) {
		emptyProfile := &profilev1.Profile{
			SampleType: []*profilev1.ValueType{
				{Type: 1, Unit: 2},
			},
			StringTable: []string{"", "samples", "count"},
			Sample:      []*profilev1.Sample{},
		}
		treeBytes, err := TreeFromBackendProfileSampleType(emptyProfile, -1, 0)
		require.NoError(t, err)
		tree := MustUnmarshalTree[FuntionName, FuntionNameI](treeBytes)
		assert.Equal(t, int64(0), tree.Total())
	})

	t.Run("handles profile with no sample types", func(t *testing.T) {
		noSampleTypesProfile := &profilev1.Profile{
			SampleType:  []*profilev1.ValueType{},
			StringTable: []string{""},
			Sample: []*profilev1.Sample{
				{
					LocationId: []uint64{1},
					Value:      []int64{10},
				},
			},
		}
		_, err := TreeFromBackendProfileSampleType(noSampleTypesProfile, -1, 0)
		require.Error(t, err)
	})

	t.Run("handles locations without line information (using addresses)", func(t *testing.T) {
		addressProfile := &profilev1.Profile{
			StringTable: []string{
				"",
				"samples",
				"count",
			},
			SampleType: []*profilev1.ValueType{
				{Type: 1, Unit: 2},
			},
			Location: []*profilev1.Location{
				{Id: 1, Address: 0x1000, Line: []*profilev1.Line{}},
				{Id: 2, Address: 0x2000, Line: []*profilev1.Line{}},
			},
			Sample: []*profilev1.Sample{
				{
					LocationId: []uint64{1, 2}, // 0x1000 -> 0x2000
					Value:      []int64{100},   // 100 samples
				},
			},
		}
		originalLen := len(addressProfile.StringTable)

		treeBytes, err := TreeFromBackendProfileSampleType(addressProfile, -1, 0)
		require.NoError(t, err)

		tree := MustUnmarshalTree[FuntionName, FuntionNameI](treeBytes)
		assert.Equal(t, int64(100), tree.Total())

		assert.Greater(t, len(addressProfile.StringTable), originalLen)

		// Check for specific address strings in the string table
		foundAddresses := 0
		for _, s := range addressProfile.StringTable {
			if s == "1000" || s == "2000" {
				foundAddresses++
			}
		}
		assert.Equal(t, 2, foundAddresses)
	})
}
