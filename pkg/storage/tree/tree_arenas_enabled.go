//go:build goexperiment.arenas
package tree

import (
	"arena"
	"bytes"
	"github.com/pyroscope-io/pyroscope/pkg/util"
	"sort"
)

func (t *Tree) InsertStackA(a *util.ArenaWrapper, stack [][]byte, v uint64) {
	if a == nil {
		t.InsertStack(stack, v)
		return
	}
	n := t.root
	for j := range stack {
		n.Total += v
		n = n.insertA(a, stack[j])
	}
	// Leaf.
	n.Total += v
	n.Self += v
}




func (n *treeNode) insertA(a *util.ArenaWrapper, targetLabel []byte) *treeNode {
	i := sort.Search(len(n.ChildrenNodes), func(i int) bool {
		return bytes.Compare(n.ChildrenNodes[i].Name, targetLabel) >= 0
	})
	if i > len(n.ChildrenNodes)-1 || !bytes.Equal(n.ChildrenNodes[i].Name, targetLabel) {
		l := arena.MakeSlice[byte](a.Arena, len(targetLabel), len(targetLabel))
		copy(l, targetLabel)
		child := newNodeA(a, l)
		n.ChildrenNodes = util.AppendA(n.ChildrenNodes, child, a)
		copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
		n.ChildrenNodes[i] = child
	}
	return n.ChildrenNodes[i]
}

func newNodeA(a *util.ArenaWrapper, label []byte) *treeNode {
	n := arena.New[treeNode](a.Arena)
	n.Name = label
	return n
}

func NewA(a *util.ArenaWrapper) *Tree {
	t := arena.New[Tree](a.Arena)
	t.root = newNodeA(a, nil)
	return t
	//return &Tree{
	//	root: newNode([]byte{}),
	//}
}
