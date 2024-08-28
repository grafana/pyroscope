package model

import (
	"bytes"
	"fmt"

	"github.com/grafana/pyroscope/pkg/og/structs/cappedarr"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// NewFlamegraphDiff generates a FlameGraphDiff from 2 trees.
// It also prunes the final tree based on the maxNodes parameter
// Notice that the resulting FlameGraph can't be used interchangeably with a 'single' Flamegraph
// Due to many differences:
// * Nodes
// * It's structure is different
//
//	i+0 = x offset, left  tree
//	i+1 = total   , left  tree
//	i+2 = self    , left  tree
//	i+3 = x offset, right tree
//	i+4 = total   , right tree
//	i+5 = self    , right tree
//	i+6 = index in the names array
func NewFlamegraphDiff(left, right *Tree, maxNodes int64) (*querierv1.FlameGraphDiff, error) {
	// The algorithm doesn't work properly with negative nodes
	// Although it's possible to silently drop these nodes
	// Let's fail early and analyze properly with real data when the issue happens
	err := assertPositiveTrees(left, right)
	if err != nil {
		return nil, err
	}
	leftTree, rightTree := combineTree(left, right)

	totalLeft := leftTree.root[0].total
	totalRight := rightTree.root[0].total

	res := &querierv1.FlameGraphDiff{
		Names:      []string{},
		Levels:     []*querierv1.Level{},
		Total:      totalLeft + totalRight,
		MaxSelf:    0,
		LeftTicks:  totalLeft,
		RightTicks: totalRight,
	}

	leftNodes, xLeftOffsets := leftTree.root, []int64{0}
	rghtNodes, xRghtOffsets := rightTree.root, []int64{0}
	levels := []int{0}
	var minVal int64
	if maxNodes > 0 {
		minVal = int64(combineMinValues(leftTree, rightTree, int(maxNodes)))
	}
	nameLocationCache := map[string]int{}

	for len(leftNodes) > 0 {
		left, rght := leftNodes[0], rghtNodes[0]
		leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]
		xLeftOffset, xRghtOffset := xLeftOffsets[0], xRghtOffsets[0]
		xLeftOffsets, xRghtOffsets = xLeftOffsets[1:], xRghtOffsets[1:]

		level := levels[0]
		levels = levels[1:]

		name := left.name
		if left.total >= minVal || rght.total >= minVal || name == "other" {
			i, ok := nameLocationCache[name]
			if !ok {
				i = len(res.Names)
				nameLocationCache[name] = i
				if i == 0 {
					name = "total"
				}

				res.Names = append(res.Names, name)
			}

			if level == len(res.Levels) {
				res.Levels = append(res.Levels, &querierv1.Level{})
			}
			res.MaxSelf = max(res.MaxSelf, left.self)
			res.MaxSelf = max(res.MaxSelf, rght.self)

			// i+0 = x offset, left  tree
			// i+1 = total   , left  tree
			// i+2 = self    , left  tree
			// i+3 = x offset, right tree
			// i+4 = total   , right tree
			// i+5 = self    , right tree
			// i+6 = index in the names array
			values := []int64{
				xLeftOffset, left.total, left.self,
				xRghtOffset, rght.total, rght.self,
				int64(i),
			}

			res.Levels[level].Values = append(values, res.Levels[level].Values...)
			xLeftOffset += left.self
			xRghtOffset += rght.self
			otherLeftTotal, otherRghtTotal := int64(0), int64(0)

			// both left and right must have the same number of children nodes
			for ni := range left.children {
				leftNode, rghtNode := left.children[ni], rght.children[ni]
				if leftNode.total >= minVal || rghtNode.total >= minVal {
					levels = prependInt(levels, level+1)
					xLeftOffsets = prependInt64(xLeftOffsets, xLeftOffset)
					xRghtOffsets = prependInt64(xRghtOffsets, xRghtOffset)
					leftNodes = prependTreeNode(leftNodes, leftNode)
					rghtNodes = prependTreeNode(rghtNodes, rghtNode)
					xLeftOffset += leftNode.total
					xRghtOffset += rghtNode.total
				} else {
					otherLeftTotal += leftNode.total
					otherRghtTotal += rghtNode.total
				}
			}
			if otherLeftTotal != 0 || otherRghtTotal != 0 {
				levels = prependInt(levels, level+1)
				{
					leftNode := &node{
						name:  "other",
						total: otherLeftTotal,
						self:  otherLeftTotal,
					}
					xLeftOffsets = prependInt64(xLeftOffsets, xLeftOffset)
					leftNodes = prependTreeNode(leftNodes, leftNode)
				}
				{
					rghtNode := &node{
						name:  "other",
						total: otherRghtTotal,
						self:  otherRghtTotal,
					}
					xRghtOffsets = prependInt64(xRghtOffsets, xRghtOffset)
					rghtNodes = prependTreeNode(rghtNodes, rghtNode)
				}
			}
		}
	}

	deltaEncoding(res.Levels, 0, 7)
	deltaEncoding(res.Levels, 3, 7)

	return res, nil
}

// combineTree aligns 2 trees by making them having the same structure with the
// same number of nodes
// It also makes the tree have a single root
func combineTree(leftTree, rightTree *Tree) (*Tree, *Tree) {
	leftTotal := int64(0)
	for _, l := range leftTree.root {
		leftTotal = leftTotal + l.total
	}

	rightTotal := int64(0)
	for _, l := range rightTree.root {
		rightTotal = rightTotal + l.total
	}

	// differently from pyroscope, there could be multiple roots
	// so we add a fake root as expected
	leftTree = &Tree{
		root: []*node{{
			children: leftTree.root,
			total:    leftTotal,
			self:     0,
		}},
	}

	rightTree = &Tree{
		root: []*node{{
			children: rightTree.root,
			total:    rightTotal,
			self:     0,
		}},
	}

	leftNodes := leftTree.root
	rghtNodes := rightTree.root

	for len(leftNodes) > 0 {
		left, rght := leftNodes[0], rghtNodes[0]

		leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]

		newLeftChildren, newRightChildren := combineNodes(left.children, rght.children)

		left.children, rght.children = newLeftChildren, newRightChildren
		leftNodes = append(leftNodes, left.children...)
		rghtNodes = append(rghtNodes, rght.children...)
	}
	return leftTree, rightTree
}

// combineNodes makes 2 slices of nodes equal
// by filling with non existing nodes
// and sorting lexicographically
func combineNodes(leftNodes, rghtNodes []*node) ([]*node, []*node) {
	size := nextPow2(maxInt(len(leftNodes), len(rghtNodes)))
	leftResult := make([]*node, 0, size)
	rghtResult := make([]*node, 0, size)

	for len(leftNodes) != 0 && len(rghtNodes) != 0 {
		left, rght := leftNodes[0], rghtNodes[0]
		switch bytes.Compare([]byte(left.name), []byte(rght.name)) {
		case 0:
			leftResult = append(leftResult, left)
			rghtResult = append(rghtResult, rght)
			leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]
		case -1:
			leftResult = append(leftResult, left)
			rghtResult = append(rghtResult, &node{name: left.name})

			leftNodes = leftNodes[1:]
		case 1:
			leftResult = append(leftResult, &node{name: rght.name})
			rghtResult = append(rghtResult, rght)
			rghtNodes = rghtNodes[1:]
		}
	}
	leftResult = append(leftResult, leftNodes...)
	rghtResult = append(rghtResult, rghtNodes...)
	for _, left := range leftNodes {
		rghtResult = append(rghtResult, &node{name: left.name})
	}
	for _, rght := range rghtNodes {
		leftResult = append(leftResult, &node{name: rght.name})
	}
	return leftResult, rghtResult
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func nextPow2(a int) int {
	a--
	a |= a >> 1
	a |= a >> 2
	a |= a >> 4
	a |= a >> 8
	a |= a >> 16
	a++
	return a
}

func combineMinValues(leftTree, rightTree *Tree, maxNodes int) uint64 {
	if maxNodes < 1 {
		return 0
	}
	// Trees are combined, meaning that their structures are
	// identical, therefore the resulting tree can not have
	// more nodes than any of them.
	treeSize := leftTree.size(make([]*node, 0, defaultDFSSize))
	if treeSize <= int64(maxNodes) {
		return 0
	}
	c := cappedarr.New(maxNodes)
	combineIterateWithTotal(leftTree, rightTree, func(left uint64, right uint64) bool {
		return c.Push(maxUint64(left, right))
	})
	return c.MinValue()
}

// iterate both trees, both trees must be returned from combineTree
func combineIterateWithTotal(leftTree, rightTree *Tree, cb func(uint64, uint64) bool) {
	leftNodes, rghtNodes := leftTree.root, rightTree.root
	i := 0
	for len(leftNodes) > 0 {
		leftNode, rghtNode := leftNodes[0], rghtNodes[0]
		leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]
		i++

		// TODO: dangerous conversion
		if cb(uint64(leftNode.total), uint64(rghtNode.total)) {
			leftNodes = append(leftNode.children, leftNodes...)
			rghtNodes = append(rghtNode.children, rghtNodes...)
		}
	}
}

// isPositiveTree returns whether a tree only contain positive values
func isPositiveTree(t *Tree) bool {
	stack := Stack[*node]{}
	for _, node := range t.root {
		stack.Push(node)
	}

	for {
		current, hasMoreNodes := stack.Pop()
		if !hasMoreNodes {
			break
		}

		if current.self < 0 {
			return false
		}

		for _, child := range current.children {
			stack.Push(child)
		}
	}

	return true
}

func assertPositiveTrees(left *Tree, right *Tree) error {
	leftRes := isPositiveTree(left)
	rightRes := isPositiveTree(right)

	if !leftRes && !rightRes {
		return fmt.Errorf("both trees require only positive values")
	}

	if !leftRes {
		return fmt.Errorf("left tree require only positive values")
	}

	if !rightRes {
		return fmt.Errorf("left tree require only positive values")
	}

	return nil
}

func deltaEncoding(levels []*querierv1.Level, start, step int) {
	for _, l := range levels {
		prev := int64(0)
		for i := start; i < len(l.Values); i += step {
			l.Values[i] -= prev
			prev += l.Values[i] + l.Values[i+1]
		}
	}
}

func prependInt(s []int, x int) []int {
	s = append(s, 0)
	copy(s[1:], s)
	s[0] = x
	return s
}

func prependInt64(s []int64, x int64) []int64 {
	s = append(s, 0)
	copy(s[1:], s)
	s[0] = x
	return s
}

func prependTreeNode(s []*node, x *node) []*node {
	s = append(s, nil)
	copy(s[1:], s)
	s[0] = x
	return s
}
