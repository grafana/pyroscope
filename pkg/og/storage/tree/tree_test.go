package tree

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTree(t *testing.T) {
	t.Run("Insert properly sets up a tree", func(t *testing.T) {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;b 1\na;c 2\n", tree.String())
	})

	t.Run("Diff properly sets up a tree", func(t *testing.T) {
		a := New()
		a.Insert([]byte("a;b;c"), uint64(100))
		a.Insert([]byte("a;b;c;d"), uint64(100))
		a.Insert([]byte("a;b;d"), uint64(100))
		a.Insert([]byte("a;e"), uint64(100))
		a.Insert([]byte("a;f"), uint64(150))
		a.Insert([]byte("a;h"), uint64(150))

		b := New()
		b.Insert([]byte("a;b;c"), uint64(120))
		b.Insert([]byte("a;b;c;d"), uint64(120))
		b.Insert([]byte("a;b;d"), uint64(120))
		b.Insert([]byte("a;e"), uint64(100))
		b.Insert([]byte("a;f"), uint64(150))
		b.Insert([]byte("a;g"), uint64(20))
		b.Insert([]byte("a;h"), uint64(170))

		diff := a.Diff(b)
		requireTreeString(t, []stack{
			{"a;g", 20},
			{"a;h", 20},
			{"a;b;c", 20},
			{"a;b;d", 20},
			{"a;b;c;d", 20},
		}, diff)
	})

	t.Run("InsertStackString unsorted of length 1", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "a"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;a 2\na;b 1\n", tree.String())
	})

	t.Run("InsertStackString equal of length 1", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "b"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 1)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, "a;b 3\n", tree.String())
	})

	t.Run("InsertStackString sorted of length 1", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "c"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;b 1\na;c 2\n", tree.String())
	})

	t.Run("InsertStackString sorted of different lengths", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "ba"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;b 1\na;ba 2\n", tree.String())
	})

	t.Run("InsertStackString unsorted of different lengths", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "ba"}, uint64(1))
		tree.InsertStackString([]string{"a", "b"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;b 2\na;ba 1\n", tree.String())
	})

	t.Run("InsertStackString unsorted of length 2", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "bb"}, uint64(1))
		tree.InsertStackString([]string{"a", "ba"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;ba 2\na;bb 1\n", tree.String())
	})

	t.Run("InsertStackString equal of length 2", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "bb"}, uint64(1))
		tree.InsertStackString([]string{"a", "bb"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 1)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, "a;bb 3\n", tree.String())
	})

	t.Run("InsertStackString sorted of length 2", func(t *testing.T) {
		tree := New()
		tree.InsertStackString([]string{"a", "bb"}, uint64(1))
		tree.InsertStackString([]string{"a", "bc"}, uint64(2))

		require.Len(t, tree.root.ChildrenNodes, 1)
		require.Len(t, tree.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), tree.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(3), tree.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(1), tree.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), tree.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, "a;bb 1\na;bc 2\n", tree.String())
	})

	t.Run("Merge similar trees", func(t *testing.T) {
		treeA := New()
		treeA.Insert([]byte("a;b"), uint64(1))
		treeA.Insert([]byte("a;c"), uint64(2))
		require.Equal(t, treeStr(`a;b 1|a;c 2|`), treeA.String())

		treeB := New()
		treeB.Insert([]byte("a;b"), uint64(4))
		treeB.Insert([]byte("a;c"), uint64(8))
		require.Equal(t, treeStr(`a;b 4|a;c 8|`), treeB.String())

		treeA.Merge(treeB)

		require.Len(t, treeA.root.ChildrenNodes, 1)
		require.Len(t, treeA.root.ChildrenNodes[0].ChildrenNodes, 2)
		require.Equal(t, uint64(0), treeA.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(15), treeA.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(5), treeA.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(10), treeA.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(5), treeA.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(10), treeA.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, treeStr(`a;b 5|a;c 10|`), treeA.String())
	})

	t.Run("Merge tree with an extra node", func(t *testing.T) {
		treeA := New()
		treeA.Insert([]byte("a;b"), uint64(1))
		treeA.Insert([]byte("a;c"), uint64(2))
		treeA.Insert([]byte("a;e"), uint64(3))
		require.Equal(t, treeStr(`a;b 1|a;c 2|a;e 3|`), treeA.String())

		treeB := New()
		treeB.Insert([]byte("a;b"), uint64(4))
		treeB.Insert([]byte("a;d"), uint64(8))
		treeB.Insert([]byte("a;e"), uint64(12))
		require.Equal(t, treeStr(`a;b 4|a;d 8|a;e 12|`), treeB.String())

		treeA.Merge(treeB)

		require.Len(t, treeA.root.ChildrenNodes, 1)
		require.Len(t, treeA.root.ChildrenNodes[0].ChildrenNodes, 4)
		require.Equal(t, uint64(0), treeA.root.ChildrenNodes[0].Self)
		require.Equal(t, uint64(30), treeA.root.ChildrenNodes[0].Total)
		require.Equal(t, uint64(5), treeA.root.ChildrenNodes[0].ChildrenNodes[0].Self)
		require.Equal(t, uint64(2), treeA.root.ChildrenNodes[0].ChildrenNodes[1].Self)
		require.Equal(t, uint64(8), treeA.root.ChildrenNodes[0].ChildrenNodes[2].Self)
		require.Equal(t, uint64(15), treeA.root.ChildrenNodes[0].ChildrenNodes[3].Self)
		require.Equal(t, uint64(5), treeA.root.ChildrenNodes[0].ChildrenNodes[0].Total)
		require.Equal(t, uint64(2), treeA.root.ChildrenNodes[0].ChildrenNodes[1].Total)
		require.Equal(t, uint64(8), treeA.root.ChildrenNodes[0].ChildrenNodes[2].Total)
		require.Equal(t, uint64(15), treeA.root.ChildrenNodes[0].ChildrenNodes[3].Total)
		require.Equal(t, treeStr(`a;b 5|a;c 2|a;d 8|a;e 15|`), treeA.String())
	})

	t.Run("tree scale", func(t *testing.T) {
		treeA := New()
		treeA.Insert([]byte("a;b"), uint64(1))
		treeA.Insert([]byte("a;c"), uint64(2))
		treeA.Insert([]byte("a;e"), uint64(3))
		treeA.Insert([]byte("a"), uint64(4))
		treeA.Scale(3)
		require.Equal(t, treeStr(`a 12|a;b 3|a;c 6|a;e 9|`), treeA.String())
	})
}

func treeStr(s string) string {
	return strings.ReplaceAll(s, "|", "\n")
}

func TestPrepend(t *testing.T) {
	t.Run("prependTreeNode", func(t *testing.T) {
		A, B, C, X := &treeNode{}, &treeNode{}, &treeNode{}, &treeNode{}
		s := []*treeNode{A, B, C}
		s = prependTreeNode(s, X)
		require.Len(t, s, 4)
		require.Equal(t, X, s[0])
		require.Equal(t, A, s[1])
		require.Equal(t, B, s[2])
		require.Equal(t, C, s[3])
	})

	t.Run("prependBytes", func(t *testing.T) {
		A, B, C, X := []byte("A"), []byte("B"), []byte("C"), []byte("X")
		s := [][]byte{A, B, C}
		s = prependBytes(s, X)

		out := bytes.Join(s, []byte(","))
		require.Equal(t, "X,A,B,C", string(out))
	})
}

type stack struct {
	Name  string
	Value int
}

func treeStringFromStacks(stacks []stack) string {
	var b strings.Builder
	for _, s := range stacks {
		_, _ = fmt.Fprintf(&b, "%s %d\n", s.Name, s.Value)
	}
	return b.String()
}

func requireTreeString(t *testing.T, stacks []stack, tr *Tree) {
	t.Helper()
	require.Equal(t, treeStringFromStacks(stacks), tr.String())
}
