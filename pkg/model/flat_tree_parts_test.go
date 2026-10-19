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
			switch r.Intn(3) {
			case 0:
				_, c := randomStacktraceTree(r, r.Intn(60), 1+r.Intn(8), callTreeNames)
				expected.MergeCallTree(c, maxNodes)
				parts.MergePreparedCallTree(PrepareCallTree(c, maxNodes))
			case 1:
				b := randomTree(r, r.Intn(60), 1+r.Intn(8), testNames).Bytes(maxNodes, nil)
				require.NoError(t, expected.MergeBytes(b))
				src := NewFlatTree()
				require.NoError(t, src.MergeBytes(b))
				parts.MergeTree(src)
			default:
				src := NewFlatTree()
				require.NoError(t, src.MergeBytes(randomTree(r, r.Intn(60), 1+r.Intn(8), testNames).Bytes(0, nil)))
				b, names := src.MarshalTruncateCompact(maxNodes)
				require.NoError(t, expected.MergeCompactBytes(b, names))
				require.NoError(t, parts.MergeCompactBytes(b, names))
			}
		}
		name := fmt.Sprintf("iter=%d inputs=%d parts=%d maxNodes=%d", iter, inputs, len(parts.parts), maxNodes)
		eb, en := expected.BytesCompact(maxNodes)
		pb, pn := parts.BytesCompact(maxNodes)
		require.Equal(t, compactTreeBytes(t, eb, en), compactTreeBytes(t, pb, pn), name)
		actual := parts.Tree()
		require.Equal(t, expected.Inputs(), actual.Inputs(), name)
		require.Equal(t, expected.Len(), actual.Len(), name)
		require.Equal(t, expected.Bytes(maxNodes), actual.Bytes(maxNodes), name)
		ab, an := actual.BytesCompact(maxNodes)
		require.Equal(t, eb, ab, name)
		require.Equal(t, en, an, name)
	}
}

func compactTreeBytes(t *testing.T, b []byte, names []string) []byte {
	tree := NewFlatTree()
	require.NoError(t, tree.MergeCompactBytes(b, names))
	return tree.Bytes(0)
}
