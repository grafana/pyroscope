package model

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	dvarint "github.com/dennwc/varint"
	"github.com/xlab/treeprint"

	"github.com/grafana/pyroscope/pkg/og/util/varint"
	"github.com/grafana/pyroscope/pkg/slices"
	"github.com/grafana/pyroscope/pkg/util/minheap"
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
	if v <= 0 {
		return
	}
	r := &node{children: t.root}
	n := r
	for s := range stack {
		name := stack[s]
		n.total += v
		// Inlined node.insert
		i, j := 0, len(n.children)
		for i < j {
			h := int(uint(i+j) >> 1)
			if n.children[h].name < name {
				i = h + 1
			} else {
				j = h
			}
		}
		if i < len(n.children) && n.children[i].name == name {
			n = n.children[i]
		} else {
			child := &node{parent: n, name: name}
			n.children = append(n.children, child)
			copy(n.children[i+1:], n.children[i:])
			n.children[i] = child
			n = child
		}
	}
	// Leaf.
	n.total += v
	n.self += v
	t.root = r.children
}

func (t *Tree) WriteCollapsed(dst io.Writer) {
	t.IterateStacks(func(_ string, self int64, stack []string) {
		slices.Reverse(stack)
		_, _ = fmt.Fprintf(dst, "%s %d\n", strings.Join(stack, ";"), self)
	})
}

func (t *Tree) IterateStacks(cb func(name string, self int64, stack []string)) {
	nodes := make([]*node, len(t.root), 1024)
	stack := make([]string, 0, 64)
	copy(nodes, t.root)
	for len(nodes) > 0 {
		n := nodes[0]
		self := n.self
		label := n.name
		if self > 0 {
			current := n
			stack = stack[:0]
			for current != nil && current.parent != nil {
				stack = append(stack, current.name)
				current = current.parent
			}
			cb(label, self, stack)
		}
		nodes = nodes[1:]
		nodes = append(nodes, n.children...)
	}
}

// Default Depth First Search slice capacity. The value should be equal
// to the number of all the siblings of the tree leaf ascendants.
//
// Chosen empirically. For very deep stacks (>128), it's likely that the
// slice will grow to 1-4K nodes, depending on the trace branching.
const defaultDFSSize = 128

func (t *Tree) Merge(src *Tree) {
	if t.Total() == 0 && src.Total() > 0 {
		*t = *src
		return
	}
	if src.Total() == 0 {
		return
	}

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

func (t *Tree) FormatNodeNames(fn func(string) string) {
	nodes := make([]*node, 0, defaultDFSSize)
	nodes = append(nodes, &node{children: t.root})
	var n *node
	var fix bool
	for len(nodes) > 0 {
		n, nodes = nodes[len(nodes)-1], nodes[:len(nodes)-1]
		m := n.name
		n.name = fn(m)
		if m != n.name {
			fix = true
		}
		nodes = append(nodes, n.children...)
	}
	if !fix {
		return
	}
	t.Fix()
}

// Fix re-establishes order of nodes and merges duplicates.
func (t *Tree) Fix() {
	if len(t.root) == 0 {
		return
	}
	r := &node{children: t.root}
	for _, n := range r.children {
		n.parent = r
	}
	nodes := make([][]*node, 0, defaultDFSSize)
	nodes = append(nodes, r.children)
	var n []*node
	for len(nodes) > 0 {
		n, nodes = nodes[len(nodes)-1], nodes[:len(nodes)-1]
		if len(n) == 0 {
			continue
		}
		sort.Slice(n, func(i, j int) bool {
			return n[i].name < n[j].name
		})
		p := n[0]
		j := 1
		for _, c := range n[1:] {
			if p.name == c.name {
				for _, x := range c.children {
					x.parent = p
				}
				p.children = append(p.children, c.children...)
				p.total += c.total
				p.self += c.self
				continue
			}
			p = c
			n[j] = c
			j++
		}
		n = n[:j]
		for _, c := range n {
			c.parent.children = n
			nodes = append(nodes, c.children)
		}
	}
	t.root = r.children
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
	nodes := make([]*node, 0, max(int64(len(t.root)), defaultDFSSize))
	treeSize := t.size(nodes)
	if treeSize <= maxNodes {
		return 0
	}

	h := make([]int64, 0, maxNodes)

	nodes = append(nodes[:0], t.root...)
	var n *node
	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]
		if len(h) >= int(maxNodes) {
			if n.total > h[0] {
				h = minheap.Pop(h)
			} else {
				continue
			}
		}
		h = minheap.Push(h, n.total)
		nodes = append(nodes, n.children...)
	}

	if len(h) < int(maxNodes) {
		return 0
	}

	return h[0]
}

// size reports number of nodes the tree consists of.
// Provided buffer used for DFS traversal.
func (t *Tree) size(buf []*node) int64 {
	nodes := append(buf, t.root...)
	var s int64
	var n *node
	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]
		nodes = append(nodes, n.children...)
		s++
	}
	return s
}

const truncatedNodeName = "other"

var truncatedNodeNameBytes = []byte(truncatedNodeName)

// Bytes returns marshaled tree byte representation; the number of nodes
// is limited to maxNodes. The function modifies the tree: truncated nodes
// are removed from the tree in place.
func (t *Tree) Bytes(maxNodes int64) []byte {
	var buf bytes.Buffer
	_ = t.MarshalTruncate(&buf, maxNodes)
	return buf.Bytes()
}

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
			if cn.total >= minVal || cn.name == truncatedNodeName {
				n.children[j] = cn
				j++
			} else {
				other += cn.total
			}
		}

		n.children = n.children[:j]
		if other > 0 {
			o := n.insert(truncatedNodeName)
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

func (m *TreeMerger) MergeTree(t *Tree) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t != nil {
		m.t.Merge(t)
	} else {
		m.t = t
	}
}

func (m *TreeMerger) MergeTreeBytes(b []byte) error {
	// TODO(kolesnikovae): Ideally, we should not have
	// the intermediate tree t but update m.t reading
	// raw bytes b directly.
	t, err := UnmarshalTree(b)
	if err != nil {
		return err
	}
	m.MergeTree(t)
	return nil
}

func (m *TreeMerger) Tree() *Tree {
	if m.t == nil {
		return new(Tree)
	}
	return m.t
}
