package tree

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
)

const (
	separator = byte(';')

	initialNodesBufferSizeCount  = 128
	initialLabelsBufferSizeBytes = 512

	positionLengthMask = 1<<32 - 1
	positionOffsetMask = positionLengthMask << 32
)

var treePool = sync.Pool{New: func() interface{} {
	return &Tree{
		labels: make([]byte, 0, initialLabelsBufferSizeBytes),
		nodes:  make([]treeNode, 1, initialNodesBufferSizeCount), // 1 for the root.
	}
}}

type Tree struct {
	sync.RWMutex
	nodes  []treeNode
	labels []byte
}

type treeNode struct {
	labelPosition uint64
	Total         uint64
	Self          uint64
	ChildrenNodes []int
}

func New() *Tree {
	return treePool.Get().(*Tree)
}

func (t *Tree) Reset() {
	t.labels = t.labels[:0]
	t.nodes = t.nodes[:0]
	treePool.Put(t)
}

func (t *Tree) newNode(label []byte) int {
	return t.put(treeNode{labelPosition: t.insertLabel(label)})
}

// put appends the given node to the tree nodes. Note that in case
// if the append causes an allocation of a new slice, existing node
// pointer should be invalidated.
//
// Check capacity before and grow tree accordingly.
func (t *Tree) put(n treeNode) int {
	t.nodes = append(t.nodes, n)
	return len(t.nodes) - 1
}

// grow increases nodes capacity slice by maximum of n and 2 * cap.
func (t *Tree) grow(n int) int {
	if n < cap(t.nodes) {
		n = cap(t.nodes)
	}
	p := t.nodes
	t.nodes = make([]treeNode, len(t.nodes), cap(t.nodes)+n)
	return copy(t.nodes, p)
}

// Note that if nodes slice is re-allocated (e.g. due to put or grow),
// the pointer will be invalid.
func (t *Tree) at(idx int) *treeNode {
	return &(t.nodes)[idx]
}

func (t *Tree) insertLabel(v []byte) uint64 {
	offset := len(t.labels) << 32
	t.labels = append(t.labels, v...)
	return uint64(offset | (len(t.labels) & positionLengthMask))
}

func (t *Tree) loadNodeLabel(idx int) []byte {
	return t.loadLabel(t.at(idx).labelPosition)
}

func (t *Tree) loadLabel(k uint64) []byte {
	return t.labels[((k & positionOffsetMask) >> 32):(k & positionLengthMask)]
}

func (t *Tree) Merge(src *Tree) {
	srcNodes := make([]int, 1, 128)
	dstNodes := make([]int, 1, 128)
	if cap(t.nodes)-len(t.nodes) < len(src.nodes) {
		t.grow(len(src.nodes))
	}

	for len(srcNodes) > 0 {
		st := src.at(srcNodes[0])
		srcNodes = srcNodes[1:]

		dt := t.at(dstNodes[0])
		dstNodes = dstNodes[1:]

		dt.Self += st.Self
		dt.Total += st.Total

		for _, sci := range st.ChildrenNodes {
			_, dci := dt.insert(t, src.loadNodeLabel(sci))
			srcNodes = append(srcNodes, sci)
			dstNodes = append(dstNodes, dci)
		}
	}
}

func (t *Tree) Insert(key []byte, value uint64, _ ...bool) {
	// It is important to grow tree before any node pointer
	// taken. Otherwise, those are invalidated, if the node
	// slice is changed.
	// TODO: find a better way.
	labels := bytes.Split(key, []byte{separator})
	if cap(t.nodes)-len(t.nodes) < len(labels) {
		t.grow(len(labels))
	}

	node := t.at(0)
	for _, label := range labels {
		node.Total += value
		node, _ = node.insert(t, label)
	}

	node.Self += value
	node.Total += value
}

func (n *treeNode) insert(t *Tree, targetLabel []byte) (*treeNode, int) {
	i := sort.Search(len(n.ChildrenNodes), func(i int) bool {
		return bytes.Compare(t.loadNodeLabel(n.ChildrenNodes[i]), targetLabel) >= 0
	})

	if i > len(n.ChildrenNodes)-1 || !bytes.Equal(t.loadNodeLabel(n.ChildrenNodes[i]), targetLabel) {
		child := t.newNode(targetLabel)
		n.ChildrenNodes = append(n.ChildrenNodes, child)
		copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
		n.ChildrenNodes[i] = child
	}

	i = n.ChildrenNodes[i]
	return t.at(i), i
}

func (t *Tree) Iterate(cb func(key []byte, val uint64)) {
	// TODO: simplify.
	nodes := []int{0}
	prefixes := make([][]byte, 1)
	prefixes[0] = make([]byte, 0)
	for len(nodes) > 0 {
		node := t.at(nodes[0])
		nodes = nodes[1:]

		prefix := prefixes[0]
		prefixes = prefixes[1:]

		label := append(prefix, separator) // byte(';'),
		l := t.loadLabel(node.labelPosition)
		label = append(label, l...) // byte(';'),

		cb(label, node.Self)

		nodes = append(node.ChildrenNodes, nodes...)
		for i := 0; i < len(node.ChildrenNodes); i++ {
			prefixes = prependBytes(prefixes, label)
		}
	}
}

func prependBytes(s [][]byte, x []byte) [][]byte {
	s = append(s, nil)
	copy(s[1:], s)
	s[0] = x
	return s
}

func (t *Tree) Samples() uint64 {
	return t.at(0).Total
}

func (t *Tree) Clone(r *big.Rat) *Tree {
	t.RLock()
	defer t.RUnlock()
	m := uint64(r.Num().Int64())
	d := uint64(r.Denom().Int64())
	return t.clone(m, d)
}

func (t *Tree) clone(m, d uint64) *Tree {
	newTrie := New()
	if s := cap(t.nodes) - cap(newTrie.nodes); s > 0 {
		newTrie.grow(s)
	}
	copy(newTrie.nodes, t.nodes)
	if s := cap(t.labels) - cap(newTrie.labels); s > 0 {
		newTrie.labels = make([]byte, len(t.labels))
	}
	copy(newTrie.labels, t.labels)
	return newTrie
}

func (t *Tree) iterateWithTotal(cb func(total uint64) bool) {
	nodes := []int{0}
	i := 0
	for len(nodes) > 0 {
		node := t.at(nodes[0])
		nodes = nodes[1:]
		i++
		if cb(node.Total) {
			nodes = append(node.ChildrenNodes, nodes...)
		}
	}
}

func (t *Tree) String() string {
	t.RLock()
	defer t.RUnlock()
	var b strings.Builder
	t.Iterate(func(k []byte, v uint64) {
		if v > 0 {
			_, _ = fmt.Fprintf(&b, "%q %d\n", k[2:], v)
		}
	})
	return b.String()
}
