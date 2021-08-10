package tree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/structs/merge"
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

var (
	placeholderTreeNode = &treeNode{}
	semicolon           = byte(';')
)

type Tree struct {
	sync.RWMutex
	root *treeNode
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

func (n *treeNode) insert(targetLabel []byte) *treeNode {
	i := sort.Search(len(n.ChildrenNodes), func(i int) bool {
		return bytes.Compare(n.ChildrenNodes[i].Name, targetLabel) >= 0
	})

	if i > len(n.ChildrenNodes)-1 || !bytes.Equal(n.ChildrenNodes[i].Name, targetLabel) {
		child := newNode(targetLabel)
		n.ChildrenNodes = append(n.ChildrenNodes, child)
		copy(n.ChildrenNodes[i+1:], n.ChildrenNodes[i:])
		n.ChildrenNodes[i] = child
	}
	return n.ChildrenNodes[i]
}

func (t *Tree) Insert(key []byte, value uint64, _ ...bool) {
	// TODO: can optimize this, split is not necessary?
	labels := bytes.Split(key, []byte(";"))
	node := t.root
	for _, l := range labels {
		buf := make([]byte, len(l))
		copy(buf, l)
		l = buf

		n := node.insert(l)

		node.Total += value
		node = n
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
		l := node.Name
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
