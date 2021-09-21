package tree

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
	"sync"
)

type treeNode struct {
	labelPosition uint64
	Total         uint64
	Self          uint64
	ChildrenNodes []*treeNode
}

func (n *treeNode) clone(dstTree, srcTree *Tree, m, d uint64) *treeNode {
	newNode := &treeNode{
		labelPosition: dstTree.insertLabel(srcTree.loadLabel(n.labelPosition)),
		Total:         n.Total * m / d,
		Self:          n.Self * m / d,
	}
	newNode.ChildrenNodes = make([]*treeNode, len(n.ChildrenNodes))
	for i, cn := range n.ChildrenNodes {
		newNode.ChildrenNodes[i] = cn.clone(dstTree, srcTree, m, d)
	}
	return newNode
}

func (t *Tree) newNode(label []byte) *treeNode {
	return &treeNode{labelPosition: t.insertLabel(label)}
}

const semicolon = byte(';')

type Tree struct {
	sync.RWMutex
	root   *treeNode
	labels []byte
}

const (
	initialLabelsBufferSizeBytes = 512

	lengthMask = 1<<32 - 1
	offsetMask = lengthMask << 32
)

func New() *Tree {
	return &Tree{
		root:   new(treeNode),
		labels: make([]byte, 0, initialLabelsBufferSizeBytes),
	}
}

func (t *Tree) insertLabel(v []byte) uint64 {
	offset := len(t.labels) << 32
	t.labels = append(t.labels, v...)
	return uint64(offset | (len(t.labels) & lengthMask))
}

func (t *Tree) loadLabel(k uint64) []byte {
	return t.labels[((k & offsetMask) >> 32):(k & lengthMask)]
}

func (t *Tree) Merge(src *Tree) {
	srcNodes := make([]*treeNode, 0, 128)
	srcNodes = append(srcNodes, src.root)

	dstNodes := make([]*treeNode, 0, 128)
	dstNodes = append(dstNodes, t.root)

	for len(srcNodes) > 0 {
		st := srcNodes[0]
		srcNodes = srcNodes[1:]

		dt := dstNodes[0]
		dstNodes = dstNodes[1:]

		dt.Self += st.Self
		dt.Total += st.Total

		for _, srcChildNode := range st.ChildrenNodes {
			dstChildNode := dt.insert(t, src.loadLabel(srcChildNode.labelPosition))
			srcNodes = prependTreeNode(srcNodes, srcChildNode)
			dstNodes = prependTreeNode(dstNodes, dstChildNode)
		}
	}
}

func prependTreeNode(s []*treeNode, x *treeNode) []*treeNode {
	s = append(s, nil)
	copy(s[1:], s)
	s[0] = x
	return s
}

func prependBytes(s [][]byte, x []byte) [][]byte {
	s = append(s, nil)
	copy(s[1:], s)
	s[0] = x
	return s
}

func prependInt(s []int, x int) []int {
	s = append(s, 0)
	copy(s[1:], s)
	s[0] = x
	return s
}

func (t *Tree) String() string {
	t.RLock()
	defer t.RUnlock()

	res := ""
	t.Iterate(func(k []byte, v uint64) {
		if v > 0 {
			res += fmt.Sprintf("%q %d\n", k[2:], v)
		}
	})

	return res
}

func (n *treeNode) insert(t *Tree, targetLabel []byte) *treeNode {
	i := sort.Search(len(n.ChildrenNodes), func(i int) bool {
		return bytes.Compare(t.loadLabel(n.ChildrenNodes[i].labelPosition), targetLabel) >= 0
	})

	if i > len(n.ChildrenNodes)-1 || !bytes.Equal(t.loadLabel(n.ChildrenNodes[i].labelPosition), targetLabel) {
		child := t.newNode(targetLabel)
		n.ChildrenNodes = append(n.ChildrenNodes, child)
		copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
		n.ChildrenNodes[i] = child
	}
	return n.ChildrenNodes[i]
}

func (t *Tree) Insert(key []byte, value uint64, _ ...bool) {
	node := t.root
	var offset int
	for i := 0; i < len(key); i++ {
		if key[i] == semicolon {
			node.Total += value
			node = node.insert(t, key[offset:i])
			offset = i + 1
		}
	}
	if offset < len(key) {
		node.Total += value
		node = node.insert(t, key[offset:])
	}
	node.Self += value
	node.Total += value
}

func (t *Tree) Iterate(cb func(key []byte, val uint64)) {
	nodes := []*treeNode{t.root}
	prefixes := make([][]byte, 1)
	prefixes[0] = make([]byte, 0)
	for len(nodes) > 0 {
		node := nodes[0]
		nodes = nodes[1:]

		prefix := prefixes[0]
		prefixes = prefixes[1:]

		label := append(prefix, semicolon) // byte(';'),
		l := t.loadLabel(node.labelPosition)
		label = append(label, l...) // byte(';'),

		cb(label, node.Self)

		nodes = append(node.ChildrenNodes, nodes...)
		for i := 0; i < len(node.ChildrenNodes); i++ {
			prefixes = prependBytes(prefixes, label)
		}
	}
}

func (t *Tree) iterateWithCum(cb func(cum uint64) bool) {
	nodes := []*treeNode{t.root}
	i := 0
	for len(nodes) > 0 {
		node := nodes[0]
		nodes = nodes[1:]
		i++
		if cb(node.Total) {
			nodes = append(node.ChildrenNodes, nodes...)
		}
	}
}

func (t *Tree) Samples() uint64 {
	return t.root.Total
}

func (t *Tree) Clone(r *big.Rat) *Tree {
	t.RLock()
	defer t.RUnlock()
	m := uint64(r.Num().Int64())
	d := uint64(r.Denom().Int64())
	newTrie := new(Tree)
	// TODO: just copy labels slice.
	newTrie.root = t.root.clone(newTrie, t, m, d)
	return newTrie
}
