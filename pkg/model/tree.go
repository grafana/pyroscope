package model

import (
	"container/heap"
	"fmt"
	"io"
	"sort"
	"sync"

	dvarint "github.com/dennwc/varint"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
	"github.com/xlab/treeprint"
)

type Tree struct {
	root []*node
}

type node struct {
	parent      *node
	children    []*node
	self, total int64
	name        string
}

func (t *Tree) String() string {
	type branch struct {
		nodes []*node
		treeprint.Tree
	}
	tree := treeprint.New()
	for _, n := range t.root {
		b := tree.AddBranch(fmt.Sprintf("%s: self %d total %d", n.name, n.self, n.total))
		remaining := append([]*branch{}, &branch{nodes: n.children, Tree: b})
		for len(remaining) > 0 {
			current := remaining[0]
			remaining = remaining[1:]
			for _, n := range current.nodes {
				if len(n.children) > 0 {
					remaining = append(remaining, &branch{nodes: n.children, Tree: current.Tree.AddBranch(fmt.Sprintf("%s: self %d total %d", n.name, n.self, n.total))})
				} else {
					current.Tree.AddNode(fmt.Sprintf("%s: self %d total %d", n.name, n.self, n.total))
				}
			}
		}
	}
	return tree.String()
}

func (t *Tree) Total() (v int64) {
	for _, n := range t.root {
		v += n.total
	}
	return v
}

func (t *Tree) InsertStack(v int64, stack ...string) {
	r := &node{children: t.root}
	n := r
	for j := range stack {
		n.total += v
		n = n.insert(stack[j])
	}
	// Leaf.
	n.total += v
	n.self += v
	t.root = r.children
}

// Default Depth First Search slice capacity. The value should be equal
// to the number of all the siblings of the tree leaf ascendants.
//
// Chosen empirically. For very deep stacks (>128), it's likely that the
// slice will grow to 1-4K nodes, depending on the trace branching.
const defaultDFSSize = 128

func (t *Tree) Merge(src *Tree) {
	srcNodes := make([]*node, 0, defaultDFSSize)
	srcRoot := &node{children: src.root}
	srcNodes = append(srcNodes, srcRoot)

	dstNodes := make([]*node, 0, defaultDFSSize)
	dstRoot := &node{children: t.root}
	dstNodes = append(dstNodes, dstRoot)

	var st, dt *node
	for len(srcNodes) > 0 {
		st, srcNodes = srcNodes[len(srcNodes)-1], srcNodes[:len(srcNodes)-1]
		dt, dstNodes = dstNodes[len(dstNodes)-1], dstNodes[:len(dstNodes)-1]

		dt.self += st.self
		dt.total += st.total

		for _, srcChildNode := range st.children {
			// Note that we don't copy the name, but reference it.
			dstChildNode := dt.insert(srcChildNode.name)
			srcNodes = append(srcNodes, srcChildNode)
			dstNodes = append(dstNodes, dstChildNode)
		}
	}

	t.root = dstRoot.children
}

func (n *node) String() string {
	return fmt.Sprintf("{%s: self %d total %d}", n.name, n.self, n.total)
}

func (n *node) insert(name string) *node {
	i := sort.Search(len(n.children), func(i int) bool {
		return n.children[i].name >= name
	})
	if i < len(n.children) && n.children[i].name == name {
		return n.children[i]
	}
	// We don't clone the name: it is caller responsibility
	// to maintain the memory ownership.
	child := &node{parent: n, name: name}
	n.children = append(n.children, child)
	copy(n.children[i+1:], n.children[i:])
	n.children[i] = child
	return child
}

// minValue returns the minimum "total" value a node in a tree has to have to show up in
// the resulting flamegraph
func (t *Tree) minValue(maxNodes int64) int64 {
	if maxNodes < 1 {
		return 0
	}

	// We should not re-use t.root capacity for nodes.
	nodes := make([]*node, len(t.root), max(int64(len(t.root)), defaultDFSSize))
	copy(nodes, t.root)
	s := make(minHeap, 0, maxNodes)
	h := &s

	var n *node
	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]
		if h.Len() >= int(maxNodes) {
			if n.total > (*h)[0] {
				heap.Pop(h)
			} else {
				continue
			}
		}
		heap.Push(h, n.total)
		nodes = append(nodes, n.children...)
	}

	if h.Len() < int(maxNodes) {
		return 0
	}

	return (*h)[0]
}

// minHeap is a custom min-heap data structure that stores integers.
type minHeap []int64

// Len returns the number of elements in the min-heap.
func (h minHeap) Len() int { return len(h) }

// Less returns true if the element at index i is less than the element at index j.
// This method is used by the container/heap package to maintain the min-heap property.
func (h minHeap) Less(i, j int) bool { return h[i] < h[j] }

// Swap exchanges the elements at index i and index j.
// This method is used by the container/heap package to reorganize the min-heap during its operations.
func (h minHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push adds an element (x) to the min-heap.
// This method is used by the container/heap package to grow the min-heap.
func (h *minHeap) Push(x interface{}) {
	*h = append(*h, x.(int64))
}

// Pop removes and returns the smallest element (minimum) from the min-heap.
// This method is used by the container/heap package to shrink the min-heap.
func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

const lostDuringSerializationName = "other"

// MarshalTruncate writes tree byte representation to the writer provider,
// the number of nodes is limited to maxNodes. The function modifies
// the tree: truncated nodes are removed from the tree.
func (t *Tree) MarshalTruncate(w io.Writer, maxNodes int64) (err error) {
	if len(t.root) == 0 {
		return nil
	}

	vw := varint.NewWriter()
	minVal := t.minValue(maxNodes)
	nodes := make([]*node, 1, defaultDFSSize)
	nodes[0] = &node{children: t.root} // Virtual root node.
	var n *node

	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]
		if _, _ = vw.Write(w, uint64(len(n.name))); err != nil {
			return err
		}
		if _, _ = w.Write(unsafeStringBytes(n.name)); err != nil {
			return err
		}
		if _, err = vw.Write(w, uint64(n.self)); err != nil {
			return err
		}

		var other int64
		var j int
		for _, cn := range n.children {
			if cn.total >= minVal || cn.name == lostDuringSerializationName {
				n.children[j] = cn
				j++
			} else {
				other += cn.total
			}
		}

		n.children = n.children[:j]
		if other > 0 {
			o := n.insert(lostDuringSerializationName)
			o.total += other
			o.self += other
		}

		if len(n.children) > 0 {
			nodes = append(nodes, n.children...)
		}
		if _, err = vw.Write(w, uint64(len(n.children))); err != nil {
			return err
		}
	}

	return nil
}

var errMalformedTreeBytes = fmt.Errorf("malformed tree bytes")

const estimateBytesPerNode = 16 // Chosen empirically.

func UnmarshalTree(b []byte) (*Tree, error) {
	t := new(Tree)
	if len(b) < 2 {
		return t, nil
	}
	size := estimateBytesPerNode
	if e := len(b) / estimateBytesPerNode; e > estimateBytesPerNode {
		size = e
	}
	parents := make([]*node, 1, size)
	// Virtual root node.
	root := new(node)
	parents[0] = root
	var parent *node
	var offset int

	for len(parents) > 0 {
		parent, parents = parents[len(parents)-1], parents[:len(parents)-1]
		nameLen, o := dvarint.Uvarint(b[offset:])
		if o < 0 {
			return nil, errMalformedTreeBytes
		}
		offset += o
		// Note that we allocate a string, instead of referencing b's capacity.
		name := string(b[offset : offset+int(nameLen)])
		offset += int(nameLen)
		value, o := dvarint.Uvarint(b[offset:])
		if o < 0 {
			return nil, errMalformedTreeBytes
		}
		offset += o
		childrenLen, o := dvarint.Uvarint(b[offset:])
		if o < 0 {
			return nil, errMalformedTreeBytes
		}
		offset += o

		n := parent.insert(name)
		n.children = make([]*node, 0, childrenLen)
		n.self = int64(value)

		pn := n
		for pn.parent != nil {
			pn.total += n.self
			pn = pn.parent
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append(parents, n)
		}
	}

	// Remove the virtual root.
	t.root = root.children[0].children

	return t, nil
}

type TreeMerger struct {
	mu sync.Mutex
	t  *Tree
}

func NewTreeMerger() *TreeMerger {
	return new(TreeMerger)
}

func (m *TreeMerger) MergeTreeBytes(b []byte) error {
	// TODO(kolesnikovae): Ideally, we should not have
	// the intermediate tree t but update m.t reading
	// raw bytes b directly.
	t, err := UnmarshalTree(b)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.t != nil {
		m.t.Merge(t)
	} else {
		m.t = t
	}
	m.mu.Unlock()
	return nil
}

func (m *TreeMerger) Tree() *Tree {
	if m.t == nil {
		return new(Tree)
	}
	return m.t
}
