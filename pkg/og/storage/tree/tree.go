package tree

import (
	"bytes"
	"encoding/json"
	"math/big"
	"sort"
	"sync"
	"unsafe"

	"github.com/grafana/pyroscope/pkg/og/util/arenahelper"

	"github.com/grafana/pyroscope/pkg/og/structs/merge"
)

type jsonableSlice []byte

type treeNode struct {
	Name          jsonableSlice `json:"name,string"`
	Total         uint64        `json:"total"`
	Self          uint64        `json:"self"`
	ChildrenNodes []*treeNode   `json:"children"`
}

func (a jsonableSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(a))
}

func (n *treeNode) clone(m, d uint64) *treeNode {
	newNode := &treeNode{
		Name:  n.Name,
		Total: n.Total * m / d,
		Self:  n.Self * m / d,
	}
	newNode.ChildrenNodes = make([]*treeNode, len(n.ChildrenNodes))
	for i, cn := range n.ChildrenNodes {
		newNode.ChildrenNodes[i] = cn.clone(m, d)
	}
	return newNode
}

func newNode(label []byte) *treeNode {
	return &treeNode{
		Name:          label,
		ChildrenNodes: []*treeNode{},
	}
}

const semicolon = byte(';')

type Tree struct {
	sync.RWMutex
	root  *treeNode
	arena arenahelper.ArenaWrapper
}

func New() *Tree {
	return &Tree{
		root: newNode([]byte{}),
	}
}

func (t *Tree) Merge(srcTrieI merge.Merger) {
	srcTrie := srcTrieI.(*Tree)

	srcNodes := make([]*treeNode, 0, 128)
	srcNodes = append(srcNodes, srcTrie.root)

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
			dstChildNode := dt.insert(srcChildNode.Name)
			srcNodes = prependTreeNode(srcNodes, srcChildNode)
			dstNodes = prependTreeNode(dstNodes, dstChildNode)
		}
	}
}

func (t *Tree) Diff(x *Tree) *Tree {
	srcNodes := make([]*treeNode, 1, 128)
	srcNodes[0] = x.root

	dstNodes := make([]*treeNode, 1, 128)
	dstNodes[0] = t.root

	for len(srcNodes) > 0 {
		sn := srcNodes[0]
		srcNodes = srcNodes[1:]

		dn := dstNodes[0]
		dstNodes = dstNodes[1:]

		if sn.Total < dn.Total || sn.Self < dn.Self {
			// src note can not be less than dst node: x always > t.
			dn.Total = 0
			dn.Self = 0
			dn.ChildrenNodes = nil
			continue
		}

		dn.Total = sn.Total - dn.Total
		dn.Self = sn.Self - dn.Self

		var d int
		for _, sc := range sn.ChildrenNodes {
			dc := dn.insert(sc.Name)
			if sc.Total == dc.Total && sc.Self == dc.Self {
				dn.removeAt(d)
				continue
			}
			dstNodes = prependTreeNode(dstNodes, dc)
			srcNodes = prependTreeNode(srcNodes, sc)
			d++
		}
		// Reclaim removed nodes space.
		for i := d; i < len(dn.ChildrenNodes); i++ {
			dn.ChildrenNodes[i] = nil
		}
		dn.ChildrenNodes = dn.ChildrenNodes[:d]
	}

	return t
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
	return t.Collapsed()
}

func (n *treeNode) insert(targetLabel []byte) *treeNode {
	i := sort.Search(len(n.ChildrenNodes), func(i int) bool {
		return bytes.Compare(n.ChildrenNodes[i].Name, targetLabel) >= 0
	})
	if i > len(n.ChildrenNodes)-1 || !bytes.Equal(n.ChildrenNodes[i].Name, targetLabel) {
		l := make([]byte, len(targetLabel))
		copy(l, targetLabel)
		child := newNode(l)
		n.ChildrenNodes = append(n.ChildrenNodes, child)
		copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
		n.ChildrenNodes[i] = child
	}
	return n.ChildrenNodes[i]
}

func (n *treeNode) removeAt(i int) {
	n.ChildrenNodes[i] = nil
	n.ChildrenNodes = append(n.ChildrenNodes[:i], n.ChildrenNodes[i+1:]...)
}

func (n *treeNode) insertString(targetLabel string) *treeNode {
	i, j := 0, len(n.ChildrenNodes)
	for i < j {
		m := (i + j) >> 1
		for k, b := range []byte(targetLabel) {
			if k >= len(n.ChildrenNodes[m].Name) || b > n.ChildrenNodes[m].Name[k] {
				// targetLabel > n.ChildrenNodes[m].Name
				i = m + 1
				break
			}
			if b < n.ChildrenNodes[m].Name[k] {
				// targetLabel < n.ChildrenNodes[m].Name
				j = m
				break
			}
			if k == len(targetLabel)-1 {
				if len(targetLabel) == len(n.ChildrenNodes[m].Name) {
					// targetLabel == n.ChildrenNodes[m].Name
					return n.ChildrenNodes[m]
				}
				// targetLabel < n.ChildrenNodes[m].Name
				j = m
			}
		}
	}
	l := []byte(targetLabel)
	child := newNode(l)
	n.ChildrenNodes = append(n.ChildrenNodes, child)
	copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
	n.ChildrenNodes[i] = child
	return n.ChildrenNodes[i]
}

func (t *Tree) InsertInt(key []byte, value int) { t.Insert(key, uint64(value)) }

func (t *Tree) Insert(key []byte, value uint64) {
	node := t.root
	var offset int
	for i, k := range key {
		if k == semicolon {
			node.Total += value
			node = node.insert(key[offset:i])
			offset = i + 1
		}
	}
	if offset < len(key) {
		node.Total += value
		node = node.insert(key[offset:])
	}
	node.Total += value
	node.Self += value
}

func (t *Tree) InsertStack(stack [][]byte, v uint64) {
	n := t.root
	for j := range stack {
		n.Total += v
		n = n.insert(stack[j])
	}
	// Leaf.
	n.Total += v
	n.Self += v
}

func (t *Tree) InsertStackString(stack []string, v uint64) {
	n := t.root
	for j := range stack {
		n.Total += v
		n = n.insertString(stack[j])
	}
	// Leaf.
	n.Total += v
	n.Self += v
}

func (t *Tree) Iterate(cb func(key []byte, val uint64)) {
	nodes := []*treeNode{t.root}
	prefixes := make([][]byte, 1)
	prefixes[0] = make([]byte, 0)
	for len(nodes) > 0 { // bfs
		node := nodes[0]
		nodes = nodes[1:]

		prefix := prefixes[0]
		prefixes = prefixes[1:]

		label := append(prefix, semicolon) // byte(';'),
		l := node.Name
		label = append(label, l...) // byte(';'),

		cb(label, node.Self)

		nodes = append(node.ChildrenNodes, nodes...)
		for i := 0; i < len(node.ChildrenNodes); i++ {
			prefixes = prependBytes(prefixes, label)
		}
	}
}

type StackBuilder interface {
	Push(frame []byte)
	Pop() // bool
	Build() (stackID uint64)
	Reset()
}

func (t *Tree) IterateWithStackBuilder(sb StackBuilder, cb func(stackID uint64, val uint64)) {
	type indexNode struct {
		index int
		node  *treeNode
	}
	var ss [128]indexNode
	s := ss[:0]
	sb.Reset()
	if t.root.Self != 0 {
		stackID := sb.Build()
		cb(stackID, t.root.Self)
	}
	for i := 0; i < len(t.root.ChildrenNodes); i++ {
		{
			c := t.root.ChildrenNodes[i]
			s = append(s, indexNode{0, c})
			sb.Push(c.Name)
			if c.Self != 0 {
				stackID := sb.Build()
				cb(stackID, c.Self)
			}
		}
		for {
			if len(s) == 0 {
				break
			}
			h := &s[len(s)-1]
			nc := len(h.node.ChildrenNodes)
			if h.index >= nc {
				s = s[0 : len(s)-1]
				sb.Pop()
				continue
			}
			c := h.node.ChildrenNodes[h.index]
			s = append(s, indexNode{0, c})
			sb.Push(c.Name)
			if c.Self != 0 {
				stackID := sb.Build()
				cb(stackID, c.Self)
			}
			h.index++
		}
	}
}

func (t *Tree) IterateStacks(cb func(name string, self uint64, stack []string)) {
	nodes := make([]*treeNode, 0, 1024)
	nodes = append(nodes, t.root)
	parents := make(map[*treeNode]*treeNode)
	stack := make([]string, 0, 64)

	for len(nodes) > 0 {
		node := nodes[0] // todo we need to chop off the last element, to avoid allocations
		self := node.Self
		label := node.nameAsStringUnsafe()
		if self > 0 {
			current := node
			stack = stack[:0]
			for current != nil && current != t.root {
				stack = append(stack, current.nameAsStringUnsafe())
				current = parents[current]
			}
			cb(label, self, stack)
		}
		nodes = nodes[1:]
		for _, child := range node.ChildrenNodes {
			nodes = append(nodes, child)
			parents[child] = node
		}
	}
}

func (n *treeNode) nameAsStringUnsafe() string {
	if len(n.Name) == 0 {
		return ""
	}
	//return unsafe.String(&n.Name[0], len(n.Name))
	res := *(*string)(unsafe.Pointer(&n.Name))
	return res
}

func (t *Tree) iterateWithTotal(cb func(total uint64) bool) {
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

func (t *Tree) Scale(s uint64) {
	nodes := make([]*treeNode, 0, 1024)
	nodes = append(nodes, t.root)

	for len(nodes) > 0 {
		node := nodes[len(nodes)-1]
		nodes = nodes[:len(nodes)-1]

		node.Self *= s
		node.Total *= s

		nodes = append(nodes, node.ChildrenNodes...)
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
	newTrie := &Tree{
		root: t.root.clone(m, d),
	}

	return newTrie
}

func (t *Tree) MarshalJSON() ([]byte, error) {
	t.RLock()
	defer t.RUnlock()
	return json.Marshal(t.root)
}
