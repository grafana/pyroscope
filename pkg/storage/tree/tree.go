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
	semicolon = byte(';')

	initialNodesBufferSizeCount  = 128
	initialLabelsBufferSizeBytes = 1 << 10 // 1 KB

	positionLengthMask = 1<<32 - 1
	positionOffsetMask = positionLengthMask << 32
)

var separator = []byte{semicolon}

type Tree struct {
	sync.RWMutex
	nodes  []treeNode
	labels *bytes.Buffer
}

type treeNode struct {
	labelPosition uint64
	Total         uint64
	Self          uint64
	ChildrenNodes []int
}

// New gets a tree from pool or creates a new one.
// The tree has already pre-allocated some space
// for node labels and nodes themselves.
//
// The tree must be Reset after use.
func New() *Tree {
	return NewSize(initialNodesBufferSizeCount)
}

func NewSize(n int) *Tree {
	return &Tree{
		labels: bytes.NewBuffer(make([]byte, 0, initialLabelsBufferSizeBytes)),
		nodes:  make([]treeNode, 0, n),
	}
}

func (t *Tree) Reset() {}

func (t *Tree) Len() int {
	return len(t.nodes)
}

func (t *Tree) Samples() uint64 {
	return t.root().Total
}

func (t *Tree) newNode(label []byte) int {
	return t.put(treeNode{labelPosition: t.insertLabel(label)})
}

// put appends the given node to the tree nodes. In case if the append
// causes an allocation of a new slice, existing node pointers are
// invalidated. Check capacity before and grow tree accordingly.
func (t *Tree) put(n treeNode) int {
	t.nodes = append(t.nodes, n)
	return len(t.nodes) - 1
}

// grow increases nodes capacity slice.
// if n < cap, then it doubles capacity.
func (t *Tree) grow(n int) int {
	if n < cap(t.nodes) {
		n = cap(t.nodes)
	}
	p := t.nodes
	t.nodes = make([]treeNode, len(t.nodes), cap(t.nodes)+n)
	return copy(t.nodes, p)
}

func (t *Tree) at(idx int) *treeNode {
	if len(t.nodes) == 0 {
		t.nodes = append(t.nodes, treeNode{})
	}
	return &(t.nodes)[idx]
}

func (t *Tree) root() *treeNode { return t.at(0) }

func (t *Tree) insertLabel(v []byte) uint64 {
	return insertLabel(t.labels, v)
}

func (t *Tree) loadNodeLabel(idx int) []byte {
	return t.loadLabel(t.at(idx).labelPosition)
}

func (t *Tree) loadLabel(k uint64) []byte {
	return t.labels.Bytes()[((k & positionOffsetMask) >> 32):(k & positionLengthMask)]
}

func insertLabel(buf *bytes.Buffer, v []byte) uint64 {
	offset := buf.Len() << 32
	_, _ = buf.Write(v)
	return uint64(offset | (buf.Len() & positionLengthMask))
}

func (t *Tree) Merge(src *Tree) {
	srcNodes := make([]int, 1, len(src.nodes)) // 1 for root.
	dstNodes := make([]int, 1, cap(t.nodes))
	// Adjust dst nodes slice capacity to make room for new nodes.
	// The resulting slice should be able to hold nodes from both trees.
	if f := cap(t.nodes) - len(t.nodes); f < len(src.nodes) {
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
			_, dci := t.insert(dt, src.loadNodeLabel(sci))
			srcNodes = append(srcNodes, sci)
			dstNodes = append(dstNodes, dci)
		}
	}
}

func (t *Tree) Insert(key []byte, value uint64, _ ...bool) {
	// It is important to grow tree before any node pointer
	// taken. Otherwise, those are invalidated, if the node
	// slice is changed.
	c := bytes.Count(key, separator) + 2
	if f := cap(t.nodes) - len(t.nodes); f < c {
		t.grow(c - f)
	}
	node := t.root()
	var offset int
	for i := 0; i < len(key); i++ {
		if key[i] == semicolon {
			node.Total += value
			node, _ = t.insert(node, key[offset:i])
			offset = i + 1
		}
	}
	if offset < len(key) {
		node.Total += value
		node, _ = t.insert(node, key[offset:])
	}
	node.Self += value
	node.Total += value
}

func (t *Tree) insert(n *treeNode, targetLabel []byte) (*treeNode, int) {
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

// Clone creates a tree copy. The copy must be reset once not used.
func (t *Tree) Clone(r *big.Rat) *Tree {
	t.RLock()
	defer t.RUnlock()
	m := uint64(r.Num().Int64())
	d := uint64(r.Denom().Int64())
	labels := make([]byte, t.labels.Len())
	copy(labels, t.labels.Bytes())
	newTrie := &Tree{
		labels: bytes.NewBuffer(labels),
		nodes:  make([]treeNode, t.Len()),
	}
	for i := range t.nodes {
		n := t.at(i)
		c := make([]int, len(n.ChildrenNodes))
		copy(c, n.ChildrenNodes)
		newTrie.nodes[i] = treeNode{
			labelPosition: n.labelPosition,
			Total:         n.Total * m / d,
			Self:          n.Self * m / d,
			ChildrenNodes: c,
		}
	}
	return newTrie
}

func (t *Tree) Iterate(cb func(key []byte, val uint64)) {
	// TODO: Refactor.
	nodes := []int{0}
	prefixes := make([][]byte, 1)
	prefixes[0] = make([]byte, 0)
	for len(nodes) > 0 {
		node := t.at(nodes[0])
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

func prependBytes(s [][]byte, x []byte) [][]byte {
	s = append(s, nil)
	copy(s[1:], s)
	s[0] = x
	return s
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

// Truncate removes nodes from t with Total value less than
// kth-smallest value. Resulting tree will only have appx.
// k most significant nodes.
func (t *Tree) Truncate(k int) {
	if t.Len() <= k {
		return
	}
	// Find kth-smallest node total value (order statistic).
	nodes := make([]uint64, len(t.nodes))
	for i := range t.nodes {
		nodes[i] = t.nodes[i].Total
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[j] < nodes[i]
	})
	// nodes slice is to be then reused for index map: only subset of t.nodes
	// will migrate to dst.nodes; consequently, node indexes will change.
	min := nodes[k]
	dstNodes := make([]treeNode, 0, k)
	labels := bytes.NewBuffer(make([]byte, 0, initialLabelsBufferSizeBytes))
	for i := range t.nodes {
		n := t.at(i)
		if n.Total < min {
			nodes[i] = 0
			n.ChildrenNodes = nil
			continue
		}
		n.labelPosition = insertLabel(labels, t.loadLabel(n.labelPosition))
		dstNodes = append(dstNodes, *n)
		nodes[i] = uint64(len(dstNodes) - 1)
	}
	// Lookup correct indexes for children nodes.
	for i := range dstNodes {
		n := dstNodes[i]
		if len(n.ChildrenNodes) == 0 {
			continue
		}
		x := 0
		for _, j := range n.ChildrenNodes {
			if newIdx := nodes[j]; newIdx > 0 {
				n.ChildrenNodes[x] = int(newIdx)
				x++
				continue
			}
			// The node was truncated, count its total as
			// parent's self.
			n.Self += t.nodes[j].Total
		}
		n.ChildrenNodes = n.ChildrenNodes[:x]
		dstNodes[i] = n
	}
	t.nodes = dstNodes
	t.labels = labels
}
