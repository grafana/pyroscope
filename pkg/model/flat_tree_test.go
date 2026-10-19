package model

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// referenceMerge merges serialized trees as the tree aggregator used to:
// a single tree is passed through, multiple trees are unmarshaled, merged,
// and serialized with truncation.
func referenceMerge(trees [][]byte, maxNodes int64) []byte {
	if len(trees) == 1 {
		return trees[0]
	}
	dst := new(FunctionNameTree)
	for _, b := range trees {
		dst.Merge(MustUnmarshalTree[FunctionName, FunctionNameI](b))
	}
	return dst.Bytes(maxNodes, nil)
}

func flatMerge(t testing.TB, trees [][]byte, maxNodes int64) *FlatTree {
	f := NewFlatTree()
	for _, b := range trees {
		require.NoError(t, f.MergeBytes(b))
	}
	return f
}

var testNames = []string{"a", "b", "c", "d", "e", "f", "g", "other", "total", "", "aa", "ab", "ba"}

func randomTree(r *rand.Rand, stacks, depth int, names []string) *FunctionNameTree {
	t := new(FunctionNameTree)
	stack := make([]FunctionName, 0, depth)
	for i := 0; i < stacks; i++ {
		stack = stack[:0]
		for d := 1 + r.Intn(depth); d > 0; d-- {
			stack = append(stack, FunctionName(names[r.Intn(len(names))]))
		}
		// Quantized values produce many ties.
		t.InsertStack(int64(1+r.Intn(4))*10, stack...)
	}
	return t
}

func Test_FlatTree_matches_FunctionNameTree(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for iter := 0; iter < 300; iter++ {
		names := testNames
		if iter%3 == 0 {
			// Without the special names.
			names = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
		}
		inputs := 1 + r.Intn(4)
		maxNodes := []int64{0, 1, 2, 5, 10, 30, 100, 1 << 20}[r.Intn(8)]
		trees := make([][]byte, inputs)
		for i := range trees {
			trees[i] = randomTree(r, 1+r.Intn(60), 1+r.Intn(8), names).Bytes(maxNodes, nil)
		}
		name := fmt.Sprintf("iter=%d inputs=%d maxNodes=%d", iter, inputs, maxNodes)

		expected := referenceMerge(trees, maxNodes)
		f := flatMerge(t, trees, maxNodes)
		require.Equal(t, expected, f.Bytes(maxNodes), name)

		for _, fgMaxNodes := range []int64{0, 3, maxNodes} {
			expectedFG := NewFlameGraph(MustUnmarshalTree[FunctionName, FunctionNameI](expected), fgMaxNodes)
			actualFG := flatMerge(t, [][]byte{expected}, fgMaxNodes).FlameGraph(fgMaxNodes)
			require.True(t, proto.Equal(expectedFG, actualFG), "%s fgMaxNodes=%d\nexpected: %v\nactual: %v", name, fgMaxNodes, expectedFG, actualFG)
		}
	}
}

func Test_FlatTree_empty(t *testing.T) {
	f := NewFlatTree()
	require.NoError(t, f.MergeBytes(nil))
	require.Nil(t, f.Bytes(10))
	require.True(t, proto.Equal(NewFlameGraph(new(FunctionNameTree), 10), f.FlameGraph(10)))
	require.Error(t, NewFlatTree().MergeBytes([]byte{5, 1}))
}

// randomStacktraceTree returns a stack trace tree of function names (the
// names are the locations) and a copy of it as a call tree.
func randomStacktraceTree(r *rand.Rand, stacks, depth int, names []string) (*StacktraceTree, *CallTree) {
	st := NewStacktraceTree(stacks * depth)
	locs := make([]int32, 0, depth)
	for i := 0; i < stacks; i++ {
		locs = locs[:0]
		for d := r.Intn(depth + 1); d > 0; d-- {
			locs = append(locs, int32(r.Intn(len(names))))
		}
		st.Insert(locs, int64(1+r.Intn(4))*10)
	}
	c := &CallTree{Names: names}
	for _, n := range st.Nodes {
		c.Parent = append(c.Parent, n.Parent)
		c.Name = append(c.Name, n.Location)
		c.Self = append(c.Self, n.Value)
		c.Total = append(c.Total, n.Total)
	}
	return st, c
}

func Test_FlatTree_MergeCallTree(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "", "total"}
	lookup := func(i int32) FunctionName { return FunctionName(names[i]) }
	for iter := 0; iter < 300; iter++ {
		inputs := 1 + r.Intn(4)
		maxNodes := []int64{0, 1, 2, 5, 10, 30, 100, 1 << 20}[r.Intn(8)]
		trees := make([][]byte, inputs)
		f := NewFlatTree()
		for i := range trees {
			st, c := randomStacktraceTree(r, r.Intn(60), 1+r.Intn(8), names)
			trees[i] = TreeFromStacktraceTree[FunctionName, FunctionNameI](st, maxNodes, lookup).Bytes(maxNodes, nil)
			f.MergeCallTree(c, maxNodes)
		}
		expected := referenceMerge(trees, maxNodes)
		require.Equal(t, expected, f.Bytes(maxNodes), "iter=%d inputs=%d maxNodes=%d", iter, inputs, maxNodes)
	}
}

func Test_kthLargest(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for iter := 0; iter < 1000; iter++ {
		s := make([]int64, r.Intn(50))
		for i := range s {
			s[i] = int64(r.Intn(10))
		}
		sorted := slices.Clone(s)
		slices.Sort(sorted)
		slices.Reverse(sorted)
		for k := 1; k <= len(s); k++ {
			require.Equal(t, sorted[k-1], kthLargest(slices.Clone(s), k))
		}
	}
}
