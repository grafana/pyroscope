package tree

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInsert(t *testing.T) {
	tree := New()
	tree.Insert([]byte("a;b"), uint64(1))
	tree.Insert([]byte("a;c"), uint64(2))

	require.Len(t, tree.root.ChildrenNodes, 1)
}
