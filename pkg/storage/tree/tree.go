package tree

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/petethepig/pyroscope/pkg/structs/merge"
)

type treeNode struct {
	// labelLink     dict.Key
	name          []byte
	cum           uint64
	self          uint64
	childrenNodes []*treeNode
}

func (n *treeNode) clone(m, d uint64) *treeNode {
	// TODO: figure out why this happens
	// if d == 0 {
	// 	d = 1
	// }
	newNode := &treeNode{
		name: n.name,
		cum:  n.cum * m / d,
		self: n.self * m / d,
	}
	newNode.childrenNodes = make([]*treeNode, len(n.childrenNodes))
	for i, cn := range n.childrenNodes {
		newNode.childrenNodes[i] = cn.clone(m, d)
	}
	return newNode
}

func newNode(label []byte) *treeNode {
	return &treeNode{
		name:          label,
		childrenNodes: []*treeNode{},
	}
}

var placeholderTreeNode = &treeNode{}
var semicolon = byte(';')

type Tree struct {
	root *treeNode
}

func New() *Tree {
	return &Tree{
		root: newNode([]byte{}),
	}
}

func (dstTrie *Tree) Merge(srcTrieI merge.Merger) {
	srcTrie := srcTrieI.(*Tree)
	srcNodes := []*treeNode{srcTrie.root}
	dstNodes := []*treeNode{dstTrie.root}

	for len(srcNodes) > 0 {
		st := srcNodes[0]
		srcNodes = srcNodes[1:]

		dt := dstNodes[0]
		dstNodes = dstNodes[1:]

		dt.self += st.self
		dt.cum += st.cum

		for _, srcChildNode := range st.childrenNodes {
			dstChildNode := dt.insert(srcChildNode.name)

			srcNodes = append([]*treeNode{srcChildNode}, srcNodes...)
			dstNodes = append([]*treeNode{dstChildNode}, dstNodes...)
		}
	}
}

func (t *Tree) String() string {
	res := ""
	t.iterate(func(k []byte, v uint64) {
		if v > 0 {
			res += fmt.Sprintf("%q %d\n", k, v)
		}
	})

	return res
}

func (tn *treeNode) insert(targetLabel []byte) *treeNode {
	i := sort.Search(len(tn.childrenNodes), func(i int) bool {
		return bytes.Compare(tn.childrenNodes[i].name, targetLabel) >= 0
	})

	if i > len(tn.childrenNodes)-1 || !bytes.Equal(tn.childrenNodes[i].name, targetLabel) {
		child := newNode(targetLabel)
		tn.childrenNodes = append(tn.childrenNodes, child)
		copy(tn.childrenNodes[i+1:], tn.childrenNodes[i:])
		tn.childrenNodes[i] = child
	}
	return tn.childrenNodes[i]
}

func (t *Tree) Insert(key []byte, value uint64, merge ...bool) {
	// TODO: can optimize this, split is not necessary?
	labels := bytes.Split(key, []byte(";"))
	node := t.root
	for _, l := range labels {
		buf := make([]byte, len(l))
		copy(buf, l)
		l = buf

		n := node.insert(l)

		node.cum += value
		node = n
	}
	node.self += value
	node.cum += value
}

// TODO: remove this
func fixLabel(v []byte) []byte {
	for i, c := range v {
		if c != byte(';') {
			return v[i:]
		}
	}
	return v
}

func (t *Tree) iterate(cb func(key []byte, val uint64)) {
	nodes := []*treeNode{t.root}
	prefixes := make([][]byte, 1)
	prefixes[0] = make([]byte, 0)
	// minVal := t.c.MinValue()
	for len(nodes) > 0 {
		node := nodes[0]
		nodes = nodes[1:]

		prefix := prefixes[0]
		prefixes = prefixes[1:]

		label := append(prefix, semicolon) // byte(';'),
		l := node.name
		label = append(label, l...) // byte(';'),

		// if node.cum > minVal {
		cb(fixLabel(label), node.self)

		nodes = append(node.childrenNodes, nodes...)
		for i := 0; i < len(node.childrenNodes); i++ {
			prefixes = append([][]byte{label}, prefixes...)
		}
		// }
	}
}

func (t *Tree) iterateWithCum(cb func(cum uint64) bool) {
	nodes := []*treeNode{t.root}
	i := 0
	for len(nodes) > 0 {
		node := nodes[0]
		nodes = nodes[1:]
		i++
		if cb(node.cum) {
			nodes = append(node.childrenNodes, nodes...)
		}
	}
}

func (t *Tree) Clone(m, d int) *Tree {
	newTrie := &Tree{
		root: t.root.clone(uint64(m), uint64(d)),
	}

	return newTrie
}
