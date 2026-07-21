package model

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"testing"
	"unique"

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
		actual, err := UnmarshalTree[FunctionName, FunctionNameI](buf.Bytes())
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
		actual, err := UnmarshalTree[FunctionName, FunctionNameI](buf.Bytes())
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

		actual, err := UnmarshalTree[FunctionName, FunctionNameI](buf.Bytes())
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
	x.FormatNodeNames(func(n FunctionName) FunctionName {
		s := string(n)
		if len(s) > 0 {
			s = s[:1]
			n = FunctionName(s)
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

func addNodeToTree(t *FunctionNameTree, name string, self, total int64) *node[FunctionName] {
	new := &node[FunctionName]{
		name:  FunctionName(name),
		self:  self,
		total: total,
	}
	t.root = append(t.root, new)
	return new
}

func addNodeToNode(n *node[FunctionName], name string, self, total int64) *node[FunctionName] {
	new := &node[FunctionName]{
		parent: n,
		name:   FunctionName(name),
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
	current := &node[FunctionName]{
		self:  stack.value,
		total: stack.value,
		name:  FunctionName(stack.locations[0]),
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

		parent := &node[FunctionName]{
			children: []*node[FunctionName]{current},
			total:    current.total,
			name:     FunctionName(name),
		}
		current.parent = parent
		current = parent
	}
	t.root = []*node[FunctionName]{current}
	return t
}

func Test_IterateStacks_LargeRootCount(t *testing.T) {
	// Test for bug fix: when tree has more than 1024 root nodes,
	// the initial capacity should be adjusted to avoid reallocation
	tree := emptyTree()

	// Create a tree with 1500 root nodes (more than default capacity of 1024)
	rootCount := 1500
	for i := 0; i < rootCount; i++ {
		tree.InsertStack(1, FunctionName("root"+strconv.Itoa(i)))
	}

	require.Equal(t, rootCount, len(tree.root), "should have %d root nodes", rootCount)

	// IterateStacks should handle this without issues
	visitedCount := 0
	tree.IterateStacks(func(name FunctionName, self int64, stack []FunctionName) {
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
		tree := MustUnmarshalTree[FunctionName, FunctionNameI](treeBytes)
		assert.Equal(t, int64(40), tree.Total())
	})

	t.Run("using second sample type (index 1)", func(t *testing.T) {
		treeBytes, err := TreeFromBackendProfileSampleType(profile, -1, 1)
		require.NoError(t, err)
		tree := MustUnmarshalTree[FunctionName, FunctionNameI](treeBytes)
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
		tree := MustUnmarshalTree[FunctionName, FunctionNameI](treeBytes)
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

		tree := MustUnmarshalTree[FunctionName, FunctionNameI](treeBytes)
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

func Test_InsertStack_SortedOrderAndMerge(t *testing.T) {
	t1 := new(FunctionNameTree)
	t1.InsertStack(1, "root", "c", "leaf1")
	t1.InsertStack(1, "root", "a", "leaf2")
	t1.InsertStack(1, "root", "b", "leaf3")

	// Children of root should be sorted ("a", "b", "c")
	require.Len(t, t1.root, 1)
	children := t1.root[0].children
	require.Len(t, children, 3)
	assert.Equal(t, FunctionName("a"), children[0].name)
	assert.Equal(t, FunctionName("b"), children[1].name)
	assert.Equal(t, FunctionName("c"), children[2].name)

	// Merge with t2
	t2 := new(FunctionNameTree)
	t2.InsertStack(2, "root", "b", "leaf3")
	t2.InsertStack(2, "root", "d", "leaf4")

	t1.Merge(t2)

	require.Len(t, t1.root, 1)
	childrenMerged := t1.root[0].children
	require.Len(t, childrenMerged, 4)
	assert.Equal(t, FunctionName("a"), childrenMerged[0].name)
	assert.Equal(t, FunctionName("b"), childrenMerged[1].name)
	assert.Equal(t, FunctionName("c"), childrenMerged[2].name)
	assert.Equal(t, FunctionName("d"), childrenMerged[3].name)
	assert.Equal(t, int64(3), childrenMerged[1].total) // b merged
}

func Test_InsertStackHandles_LocationRefTree(t *testing.T) {
	t1 := new(LocationRefNameTree)
	h1 := unique.Make("100")
	h2 := unique.Make("200")

	// Should not panic on LocationRefNameTree
	require.NotPanics(t, func() {
		t1.InsertStackHandles(5, h1, h2)
	})
	assert.Equal(t, int64(5), t1.Total())
}

func Test_InsertStackHandles_SmallFanoutDuplicatePrevention(t *testing.T) {
	t1 := new(FunctionNameTree)
	// Insert via InsertStack (handle is zero/unset)
	t1.InsertStack(10, "root", "childA")

	// Insert via InsertStackHandles (small fanout < 8)
	hRoot := unique.Make("root")
	hChildA := unique.Make("childA")
	t1.InsertStackHandles(5, hRoot, hChildA)

	require.Len(t, t1.root, 1)
	assert.Equal(t, int64(15), t1.root[0].total)
	assert.Equal(t, hRoot, t1.root[0].handle)

	// Ensure no duplicate sibling under root
	require.Len(t, t1.root[0].children, 1, "should not create duplicate child node in small fanout")
	childA := t1.root[0].children[0]
	assert.Equal(t, FunctionName("childA"), childA.name)
	assert.Equal(t, int64(15), childA.total)
	assert.Equal(t, hChildA, childA.handle)

	// Subsequent InsertStackHandles should hit by handle
	hChildB := unique.Make("childB")
	t1.InsertStackHandles(3, hRoot, hChildA, hChildB)
	assert.Equal(t, int64(18), t1.root[0].total)
	assert.Equal(t, int64(18), childA.total)
}

func Test_InsertStackHandles_LargeFanoutHandleAssignment(t *testing.T) {
	t1 := new(FunctionNameTree)

	// Insert 8 children via InsertStack (large fanout >= 8)
	hNames := make([]string, 10)
	handles := make([]unique.Handle[string], 10)
	for i := range 10 {
		hNames[i] = fmt.Sprintf("child_%02d", i)
		handles[i] = unique.Make(hNames[i])
		t1.InsertStack(10, "root", FunctionName(hNames[i]))
	}

	hRoot := unique.Make("root")

	// Verify large fanout has 10 children with zero handles
	var zeroHandle unique.Handle[string]
	require.Len(t, t1.root, 1)
	require.Len(t, t1.root[0].children, 10)
	for _, c := range t1.root[0].children {
		assert.Equal(t, zeroHandle, c.handle)
	}

	// Insert via InsertStackHandles into large fanout
	t1.InsertStackHandles(5, hRoot, handles[3])

	// The node for child_03 should now have handle assigned
	child3 := t1.root[0].children[3]
	assert.Equal(t, FunctionName("child_03"), child3.name)
	assert.Equal(t, int64(15), child3.total)
	assert.Equal(t, handles[3], child3.handle)
}

func Test_InsertStackHandles_MergeHandlePropagation(t *testing.T) {
	// t1 created via InsertStack (no handles)
	t1 := new(FunctionNameTree)
	t1.InsertStack(10, "root", "childA")

	// t2 created via InsertStackHandles (has handles)
	t2 := new(FunctionNameTree)
	hRoot := unique.Make("root")
	hChildA := unique.Make("childA")
	t2.InsertStackHandles(5, hRoot, hChildA)

	// Merge t2 into t1
	t1.Merge(t2)

	require.Len(t, t1.root, 1)
	assert.Equal(t, int64(15), t1.root[0].total)
	require.Len(t, t1.root[0].children, 1)
	assert.Equal(t, int64(15), t1.root[0].children[0].total)
	assert.Equal(t, hChildA, t1.root[0].children[0].handle, "handle should be propagated from src to dst on merge")

	// Further InsertStackHandles on merged t1 should hit handle
	t1.InsertStackHandles(2, hRoot, hChildA)
	assert.Equal(t, int64(17), t1.root[0].total)
	assert.Len(t, t1.root[0].children, 1)
}

func Test_InsertStackHandles_FixHandlePropagation(t *testing.T) {
	t1 := new(FunctionNameTree)
	hRoot := unique.Make("root")
	hChildA := unique.Make("childA")

	// FormatNodeNames / Fix test
	t1.InsertStack(10, "root", "childA_old")
	t1.InsertStackHandles(5, hRoot, hChildA)

	// Rename childA_old to childA and trigger Fix()
	t1.FormatNodeNames(func(n FunctionName) FunctionName {
		if n == "childA_old" {
			return "childA"
		}
		return n
	})

	require.Len(t, t1.root, 1)
	require.Len(t, t1.root[0].children, 1)
	assert.Equal(t, int64(15), t1.root[0].children[0].total)
	assert.Equal(t, hChildA, t1.root[0].children[0].handle, "Fix should consolidate handles when merging nodes")
}

func Test_FormatNodeNames_ClearsStaleHandle(t *testing.T) {
	t1 := new(FunctionNameTree)
	hRoot := unique.Make("root")
	hChildA := unique.Make("childA")
	hChildB := unique.Make("childB")

	// Insert stack with handles
	t1.InsertStackHandles(10, hRoot, hChildA)

	// Verify childA has handle hChildA
	require.Len(t, t1.root, 1)
	require.Len(t, t1.root[0].children, 1)
	assert.Equal(t, hChildA, t1.root[0].children[0].handle)

	// FormatNodeNames renames childA -> childB
	t1.FormatNodeNames(func(n FunctionName) FunctionName {
		if n == "childA" {
			return "childB"
		}
		return n
	})

	// The node's handle must no longer be hChildA!
	childNode := t1.root[0].children[0]
	assert.Equal(t, FunctionName("childB"), childNode.name)
	var zeroHandle unique.Handle[string]
	assert.Equal(t, zeroHandle, childNode.handle, "stale handle for old name childA should be cleared after rename")

	// Now insert childA with hChildA via InsertStackHandles
	t1.InsertStackHandles(5, hRoot, hChildA)

	// It should NOT match childB! It should create a new node for childA
	require.Len(t, t1.root[0].children, 2)
	assert.Equal(t, FunctionName("childA"), t1.root[0].children[0].name)
	assert.Equal(t, FunctionName("childB"), t1.root[0].children[1].name)
	assert.Equal(t, int64(5), t1.root[0].children[0].total)
	assert.Equal(t, int64(10), t1.root[0].children[1].total)

	// Now insert childB with hChildB via InsertStackHandles
	t1.InsertStackHandles(3, hRoot, hChildB)

	// It SHOULD match childB and assign hChildB
	assert.Equal(t, int64(13), t1.root[0].children[1].total)
	assert.Equal(t, hChildB, t1.root[0].children[1].handle)
}

func Test_InsertStackHandles_InvalidLocationRefHandle(t *testing.T) {
	t1 := new(LocationRefNameTree)
	hInvalid1 := unique.Make("invalid_a")
	hInvalid2 := unique.Make("invalid_b")
	hValid := unique.Make("123")

	// Insert invalid handles
	t1.InsertStackHandles(10, hInvalid1, hInvalid2)

	// Tree root should be empty (invalid handles skipped, not collapsed to node 0)
	assert.Len(t, t1.root, 0)
	assert.Equal(t, int64(0), t1.Total())

	// Insert valid handle
	t1.InsertStackHandles(5, hValid)
	require.Len(t, t1.root, 1)
	assert.Equal(t, LocationRefName(123), t1.root[0].name)
	assert.Equal(t, int64(5), t1.Total())
}

func Test_InsertStackHandles_MixedValidInvalidHandles(t *testing.T) {
	t1 := new(LocationRefNameTree)
	hValid1 := unique.Make("100")
	hInvalid := unique.Make("invalid_frame")
	hValid2 := unique.Make("200")

	// Mixed stack: valid, invalid, valid
	t1.InsertStackHandles(10, hValid1, hInvalid, hValid2)

	// Tree should remain empty: early rejection prevents path splicing (attaching 200 to 100)
	assert.Len(t, t1.root, 0)
	assert.Equal(t, int64(0), t1.Total())
}

type customStringName string
type customIntName int

func Test_StringToNodeName_CustomNodeNameTypes(t *testing.T) {
	// customStringName (~string)
	cs, err := stringToNodeName[customStringName]("hello")
	require.NoError(t, err)
	assert.Equal(t, customStringName("hello"), cs)

	// customIntName (~int)
	ci, err := stringToNodeName[customIntName]("456")
	require.NoError(t, err)
	assert.Equal(t, customIntName(456), ci)

	// customIntName invalid number
	_, err = stringToNodeName[customIntName]("invalid")
	require.Error(t, err)
}
