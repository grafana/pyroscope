package tree

import (
	"bytes"

	"github.com/pyroscope-io/pyroscope/pkg/structs/cappedarr"
)

// CombineTree aligns 2 trees by making them having the same structure with the
// same number of nodes
// TODO: create a new struct?
func CombineTree(leftTree, rightTree *Tree) (*Tree, *Tree) {
	leftTree.Lock()
	defer leftTree.Unlock()
	rightTree.Lock()
	defer rightTree.Unlock()

	leftNodes := []int{0}
	rightNodes := []int{0}

	for len(leftNodes) > 0 {
		left, right := leftTree.at(leftNodes[0]), rightTree.at(rightNodes[0])
		leftNodes, rightNodes = leftNodes[1:], rightNodes[1:]

		left.ChildrenNodes, right.ChildrenNodes = combineNodes(leftTree, rightTree, left.ChildrenNodes, right.ChildrenNodes)
		leftNodes = append(leftNodes, left.ChildrenNodes...)
		rightNodes = append(rightNodes, right.ChildrenNodes...)
	}
	return leftTree, rightTree
}

func combineNodes(leftTree, rightTree *Tree, leftNodes, rightNodes []int) ([]int, []int) {
	size := nextPow2(max(len(leftNodes), len(rightNodes)))
	leftResult := make([]int, 0, size)
	rightResult := make([]int, 0, size)

	for len(leftNodes) != 0 && len(rightNodes) != 0 {
		left, right := leftNodes[0], rightNodes[0]
		switch bytes.Compare(leftTree.loadNodeLabel(left), rightTree.loadNodeLabel(right)) {
		case 0:
			leftResult = append(leftResult, left)
			rightResult = append(rightResult, right)
			leftNodes, rightNodes = leftNodes[1:], rightNodes[1:]
		case -1:
			leftResult = append(leftResult, left)
			rightResult = append(rightResult, rightTree.newNode(leftTree.loadNodeLabel(left)))
			leftNodes = leftNodes[1:]
		case 1:
			leftResult = append(leftResult, leftTree.newNode(rightTree.loadNodeLabel(right)))
			rightResult = append(rightResult, right)
			rightNodes = rightNodes[1:]
		}
	}
	leftResult = append(leftResult, leftNodes...)
	rightResult = append(rightResult, rightNodes...)
	for _, left := range leftNodes {
		rightResult = append(rightResult, rightTree.newNode(leftTree.loadNodeLabel(left)))
	}
	for _, right := range rightNodes {
		leftResult = append(leftResult, leftTree.newNode(rightTree.loadNodeLabel(right)))
	}
	return leftResult, rightResult
}

// CombineToFlamebearerStruct generates the Flamebearer struct from 2 trees.
// They must be the response trees from CombineTree (i.e. all children nodes
// must be the same length). The Flamebearer struct returned from this function
// is different to the one returned from Tree.FlamebearerStruct(). It has the
// following structure:
//
//     i+0 = x offset, left  tree
//     i+1 = total   , left  tree
//     i+2 = self    , left  tree
//     i+3 = x offset, right tree
//     i+4 = total   , right tree
//     i+5 = self    , right tree
//     i+6 = index in the names array
func CombineToFlamebearerStruct(leftTree, rightTree *Tree, maxNodes int) *Flamebearer {
	leftTree.RLock()
	defer leftTree.RUnlock()
	rightTree.RLock()
	defer rightTree.RUnlock()

	res := Flamebearer{
		Names:    []string{},
		Levels:   [][]int{},
		NumTicks: int(leftTree.Samples() + rightTree.Samples()),
		MaxSelf:  int(0),
		Format:   FormatDouble,
	}

	leftNodes, xLeftOffsets := []*treeNode{leftTree.at(0)}, []int{0}
	rightNodes, xRightOffsets := []*treeNode{rightTree.at(0)}, []int{0}
	levels := []int{0}
	minVal := combineMinValues(leftTree, rightTree, maxNodes)
	nameLocationCache := map[string]int{}

	for len(leftNodes) > 0 {
		left, right := leftNodes[0], rightNodes[0]
		leftNodes, rightNodes = leftNodes[1:], rightNodes[1:]

		xLeftOffset, xRightOffset := xLeftOffsets[0], xRightOffsets[0]
		xLeftOffsets, xRightOffsets = xLeftOffsets[1:], xRightOffsets[1:]

		level := levels[0]
		levels = levels[1:]

		// both left.Name and right.Name must be the same
		name := string(rightTree.loadLabel(right.labelPosition))
		if left.Total >= minVal || right.Total >= minVal || name == "other" {
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
				res.Levels = append(res.Levels, []int{})
			}
			res.MaxSelf = max(res.MaxSelf, int(left.Self))
			res.MaxSelf = max(res.MaxSelf, int(right.Self))

			// i+0 = x offset, left  tree
			// i+1 = total   , left  tree
			// i+2 = self    , left  tree
			// i+3 = x offset, right tree
			// i+4 = total   , right tree
			// i+5 = self    , right tree
			// i+6 = index in the names array
			values := []int{
				xLeftOffset, int(left.Total), int(left.Self),
				xRightOffset, int(right.Total), int(right.Self),
				i,
			}
			res.Levels[level] = append(values, res.Levels[level]...)

			xLeftOffset += int(left.Self)
			xRightOffset += int(right.Self)
			otherLeftTotal, otherRightTotal := uint64(0), uint64(0)

			// both left and right must have the same number of children nodes
			for ni := range left.ChildrenNodes {
				leftNode, rightNode := leftTree.at(left.ChildrenNodes[ni]), rightTree.at(right.ChildrenNodes[ni])
				if leftNode.Total >= minVal || rightNode.Total >= minVal {
					levels = prependInt(levels, level+1)
					xLeftOffsets = prependInt(xLeftOffsets, xLeftOffset)
					xRightOffsets = prependInt(xRightOffsets, xRightOffset)
					leftNodes = prependTreeNode(leftNodes, leftTree.at(left.ChildrenNodes[ni]))
					rightNodes = prependTreeNode(rightNodes, rightTree.at(right.ChildrenNodes[ni]))
					xLeftOffset += int(leftNode.Total)
					xRightOffset += int(rightNode.Total)
				} else {
					otherLeftTotal += leftNode.Total
					otherRightTotal += rightNode.Total
				}
			}
			if otherLeftTotal != 0 || otherRightTotal != 0 {
				levels = prependInt(levels, level+1)
				xLeftOffsets = prependInt(xLeftOffsets, xLeftOffset)
				leftNodes = prependTreeNode(leftNodes, &treeNode{
					labelPosition: leftTree.insertLabel([]byte("other")),
					Total:         otherLeftTotal,
					Self:          otherLeftTotal,
				})
				xRightOffsets = prependInt(xRightOffsets, xRightOffset)
				rightNodes = prependTreeNode(rightNodes, &treeNode{
					labelPosition: rightTree.insertLabel([]byte("other")),
					Total:         otherRightTotal,
					Self:          otherRightTotal,
				})
			}
		}
	}

	// delta encoding
	deltaEncoding(res.Levels, 0, 7)
	deltaEncoding(res.Levels, 3, 7)
	return &res
}

func combineMinValues(leftTree, rightTree *Tree, maxNodes int) uint64 {
	c := cappedarr.New(maxNodes)
	combineIterateWithTotal(leftTree, rightTree, func(left uint64, right uint64) bool {
		return c.Push(maxUint64(left, right))
	})
	return c.MinValue()
}

// iterate both trees, both trees must be returned from CombineTree
func combineIterateWithTotal(leftTree, rightTree *Tree, cb func(uint64, uint64) bool) {
	leftNodes, rightNodes := []int{0}, []int{0}
	i := 0
	for len(leftNodes) > 0 {
		left, right := leftTree.at(leftNodes[0]), rightTree.at(rightNodes[0])
		leftNodes, rightNodes = leftNodes[1:], rightNodes[1:]
		i++
		if cb(left.Total, right.Total) {
			leftNodes = append(left.ChildrenNodes, leftNodes...)
			rightNodes = append(right.ChildrenNodes, rightNodes...)
		}
	}
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func max(a, b int) int {
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

func prependTreeNode(s []*treeNode, x *treeNode) []*treeNode {
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
