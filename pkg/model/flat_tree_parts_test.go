package model

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_FlatTreeParts_matches_FlatTree(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	callTreeNames := []string{"a", "b", "c", "d", "e", "f", "g", "h", "", "total"}
	for iter := 0; iter < 400; iter++ {
		maxNodes := []int64{0, 1, 2, 5, 10, 30, 100, 1 << 20}[r.Intn(8)]
		parts := NewFlatTreeParts(1 + r.Intn(5))
		expected := NewFlatTree()
		inputs := r.Intn(6)
		for i := 0; i < inputs; i++ {
			if r.Intn(2) == 0 {
				_, c := randomStacktraceTree(r, r.Intn(60), 1+r.Intn(8), callTreeNames)
				expected.MergeCallTree(c, maxNodes)
				parts.MergePreparedCallTree(PrepareCallTree(c, maxNodes))
				continue
			}
			b := randomTree(r, r.Intn(60), 1+r.Intn(8), testNames).Bytes(maxNodes, nil)
			require.NoError(t, expected.MergeBytes(b))
			src := NewFlatTree()
			require.NoError(t, src.MergeBytes(b))
			parts.MergeTree(src)
		}
		name := fmt.Sprintf("iter=%d inputs=%d parts=%d maxNodes=%d", iter, inputs, len(parts.parts), maxNodes)
		actual := parts.Tree()
		require.Equal(t, expected.Inputs(), actual.Inputs(), name)
		require.Equal(t, expected.Len(), actual.Len(), name)
		require.Equal(t, expected.Bytes(maxNodes), actual.Bytes(maxNodes), name)
	}
}
