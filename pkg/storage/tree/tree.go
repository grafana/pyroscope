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

	initialNodesBufferSizeCount  = 512
	initialLabelsBufferSizeBytes = 1 << 10 // 1 KB

	positionLengthMask = 1<<32 - 1
	positionOffsetMask = positionLengthMask << 32
)

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

func New() *Tree {
	return &Tree{
		labels: bytes.NewBuffer(make([]byte, 0, initialLabelsBufferSizeBytes)),
		nodes:  make([]treeNode, 1, initialNodesBufferSizeCount), // 1 for root.
	}
}

func (t *Tree) Len() int { return len(t.nodes) }

func (t *Tree) Samples() uint64 { return t.at(0).Total }

// newNode creates a new tree node and appends it to the tree nodes slice.
// In case if the append causes an allocation of a new slice, existing
// node pointers obtained with 'at' are invalidated.
func (t *Tree) newNode(label []byte) int {
	t.nodes = append(t.nodes, treeNode{labelPosition: t.insertLabel(label)})
	return len(t.nodes) - 1
}

// at references node at idx. If t.nodes slice changes due
// to re-allocation, the reference invalidates.
func (t *Tree) at(idx int) *treeNode { return &(t.nodes)[idx] }

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
	dstNodes := make([]int, 1, len(t.nodes))
	for len(srcNodes) > 0 {
		si := srcNodes[0]
		di := dstNodes[0]
		srcNodes = srcNodes[1:]
		dstNodes = dstNodes[1:]
		sn := src.at(si)
		dn := t.at(di)
		dn.Self += sn.Self
		dn.Total += sn.Total
		for _, sci := range sn.ChildrenNodes {
			dci := t.insert(di, src.loadNodeLabel(sci))
			srcNodes = append(srcNodes, sci)
			dstNodes = append(dstNodes, dci)
		}
	}
}

func (t *Tree) Insert(key []byte, value uint64, _ ...bool) {
	var idx int
	var offset int
	for i := 0; i < len(key); i++ {
		if key[i] == semicolon {
			t.nodes[idx].Total += value
			idx = t.insert(idx, key[offset:i])
			offset = i + 1
		}
	}
	if offset < len(key) {
		t.nodes[idx].Total += value
		idx = t.insert(idx, key[offset:])
	}
	t.nodes[idx].Self += value
	t.nodes[idx].Total += value
}

func (t *Tree) insert(idx int, targetLabel []byte) int {
	n := t.nodes[idx]
	i := sort.Search(len(n.ChildrenNodes), func(i int) bool {
		return bytes.Compare(t.loadNodeLabel(n.ChildrenNodes[i]), targetLabel) >= 0
	})

	if i > len(n.ChildrenNodes)-1 || !bytes.Equal(t.loadNodeLabel(n.ChildrenNodes[i]), targetLabel) {
		child := t.newNode(targetLabel)
		n.ChildrenNodes = append(n.ChildrenNodes, child)
		copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
		n.ChildrenNodes[i] = child
		t.nodes[idx].ChildrenNodes = n.ChildrenNodes
	}

	return n.ChildrenNodes[i]
}

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
		n := t.nodes[i]
		if n.Total < min {
			nodes[i] = 0
			n.ChildrenNodes = nil
			continue
		}
		n.labelPosition = insertLabel(labels, t.loadLabel(n.labelPosition))
		dstNodes = append(dstNodes, n)
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
