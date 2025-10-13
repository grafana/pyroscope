package model

import (
	"fmt"

	"github.com/grafana/pyroscope/pkg/util/minheap"
)

type AVLTree struct {
	root       *AVLNode
	totalNodes int32
}

func (t *AVLTree) Insert(locations []int32, value int64) {
	var inserted *AVLNode
	var insertedNodes int
	tree := t
	for i := len(locations) - 1; i >= 0; i-- {
		location := locations[i]
		inserted, tree.root, insertedNodes = tree.root.insert(location, value, i == 0)
		tree = inserted.children
		t.totalNodes += int32(insertedNodes)
	}
}

func (t *AVLTree) Len() int32 {
	return t.totalNodes
}

func (t *AVLTree) ToString() string {
	return t.root.ToString()
}

func (t *AVLNode) ToString() string {
	if t == nil {
		return ""
	}
	return t.left.ToString() + fmt.Sprintf("[%d %d %d]", t.Location, t.Value, t.Total) + t.children.ToString() + t.right.ToString()
}

func (t *AVLTree) Print(level int32) {
	t.root.Print(level)
}

func (t *AVLNode) Print(level int32) {
	if t == nil {
		return
	}
	t.left.Print(level)
	fmt.Printf("%d %d %d %d\n", level, t.Location, t.Value, t.Total)
	t.children.Print(level + 1)
	t.right.Print(level)
}

type AVLNode struct {
	Location int32
	Value    int64
	Total    int64
	left     *AVLNode
	right    *AVLNode
	height   int32
	children *AVLTree
}

func (n *AVLNode) updateHeight() {
	left, right := int32(-1), int32(-1)
	if n.left != nil {
		left = n.left.height
	}
	if n.right != nil {
		right = n.right.height
	}
	n.height = 1 + max(left, right)
}

func (n *AVLNode) rightRotation() *AVLNode {
	x := n.left
	n.left = x.right
	x.right = n
	n.updateHeight()
	x.updateHeight()
	return x
}

func (n *AVLNode) leftRotation() *AVLNode {
	x := n.right
	n.right = x.left
	x.left = n
	n.updateHeight()
	x.updateHeight()
	return x
}

func (n *AVLNode) balanceFactor() int32 {
	left, right := int32(-1), int32(-1)
	if n.left != nil {
		left = n.left.height
	}
	if n.right != nil {
		right = n.right.height
	}
	return left - right
}

// return the new root

func (n *AVLNode) rebalance() *AVLNode {
	bf := n.balanceFactor()
	if bf > 1 {
		// LR
		if n.left != nil && n.left.balanceFactor() < 0 {
			n.left = n.left.leftRotation()
		}
		// LL
		n = n.rightRotation()
	} else if bf < -1 {
		// RL
		if n.right != nil && n.right.balanceFactor() > 0 {
			n.right = n.right.rightRotation()
		}
		// RR
		n = n.leftRotation()
	}
	return n
}

func NewAVLNode(key int32, val int64, total int64) *AVLNode {
	return &AVLNode{
		Location: key,
		Value:    val,
		Total:    total,
		left:     nil,
		right:    nil,
		height:   0,
		children: &AVLTree{},
	}
}

func (n *AVLNode) insert(location int32, value int64, isFinal bool) (*AVLNode, *AVLNode, int) {
	var inserted *AVLNode
	if n == nil {
		total := value
		if !isFinal {
			value = 0
		}
		inserted = NewAVLNode(location, value, total)
		return inserted, inserted, 1
	}
	insertedNodes := 0
	if location < n.Location {
		inserted, n.left, insertedNodes = n.left.insert(location, value, isFinal)
	} else if location > n.Location {
		inserted, n.right, insertedNodes = n.right.insert(location, value, isFinal)
	} else {
		inserted = n
		if isFinal {
			n.Value += value
		}
		n.Total += value
	}
	n.updateHeight()
	return inserted, n.rebalance(), insertedNodes
}

func (t *AVLTree) MinValue(maxNodes int64) int64 {
	if maxNodes < 1 || maxNodes >= int64(t.totalNodes) {
		return 0
	}
	h := make([]int64, 0, maxNodes)
	for i := NewAVLIterator(*t); i.Next(); {
		curr := i.At()
		if len(h) >= int(maxNodes) {
			if curr.Total > h[0] {
				h = minheap.Pop(h)
			} else {
				continue
			}
		}
		h = minheap.Push(h, curr.Total)
	}
	return h[0]
}

func (t *AVLTree) Tree(maxNodes int64, names []string) *Tree {
	if t.totalNodes < 1 || len(names) == 0 {
		return new(Tree)
	}

	minValue := t.MinValue(maxNodes)
	root := new(node)
	children := t.root.tree(minValue, names, root)
	for _, n := range children {
		n.parent = nil
	}

	return &Tree{root: children}
}

func (n *AVLNode) tree(minValue int64, names []string, parent *node) []*node {
	i := NewAVLChildrenIterator(n)
	children := make([]*node, 0)
	truncated := int64(0)
	for i.Next() {
		avlNode := i.At()
		if avlNode.Total >= minValue && avlNode.Location >= 0 && avlNode.Location < int32(len(names)) {
			aux := &node{
				parent: parent,
				name:   names[avlNode.Location],
				self:   avlNode.Value,
				total:  avlNode.Total,
			}
			aux.children = avlNode.children.root.tree(minValue, names, aux)
			children = append(children, aux)
		} else {
			truncated += avlNode.Total
		}
	}
	if truncated > 0 {
		children = append(children, &node{
			parent: parent,
			name:   truncatedNodeName,
			self:   truncated,
			total:  truncated,
		})
	}
	return children
}

type AVLIterator struct {
	includeChildren bool

	current *AVLNode

	queue []*AVLNode
}

func NewAVLChildrenIterator(node *AVLNode) *AVLIterator {
	return &AVLIterator{
		includeChildren: false,
		current:         node,
		queue:           make([]*AVLNode, 0), // TODO?
	}
}

func NewAVLIterator(t AVLTree) *AVLIterator {
	return &AVLIterator{
		includeChildren: true,
		current:         t.root,
		queue:           make([]*AVLNode, 0, t.totalNodes+1),
	}
}

func (i *AVLIterator) Next() bool {
	return i.current != nil
}

func (i *AVLIterator) At() *AVLNode {
	at := i.current
	if at.left != nil {
		i.queue = append(i.queue, at.left)
	}
	if i.includeChildren {
		if at.children.root != nil {
			i.queue = append(i.queue, at.children.root)
		}
	}
	if at.right != nil {
		i.queue = append(i.queue, at.right)
	}
	if len(i.queue) > 0 {
		i.current, i.queue = i.queue[0], i.queue[1:]
	} else {
		i.current = nil
	}
	return at
}

func (i *AVLIterator) Err() error {
	return nil
}

func (i *AVLIterator) Close() error {
	return nil
}
