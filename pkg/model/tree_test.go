package model

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
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

func Test_FormatNames(t *testing.T) {
	x := newTree([]stacktraces{
		{locations: []string{"c0", "b0", "a0"}, value: 3},
		{locations: []string{"c1", "b0", "a0"}, value: 3},
		{locations: []string{"d0", "b1", "a0"}, value: 2},
		{locations: []string{"e1", "c1", "a1"}, value: 4},
		{locations: []string{"e2", "c1", "a2"}, value: 4},
	})
	x.FormatNodeNames(func(n string) string {
		if len(n) > 0 {
			n = n[:1]
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
		tree := MustUnmarshalTree(treeBytes)
		assert.Equal(t, int64(40), tree.Total())
	})

	t.Run("using second sample type (index 1)", func(t *testing.T) {
		treeBytes, err := TreeFromBackendProfileSampleType(profile, -1, 1)
		require.NoError(t, err)
		tree := MustUnmarshalTree(treeBytes)
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
		tree := MustUnmarshalTree(treeBytes)
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

		tree := MustUnmarshalTree(treeBytes)
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
