package tree

import (
	"bytes"

	"github.com/grafana/pyroscope/pkg/og/structs/cappedarr"
)

// CombineTree aligns 2 trees by making them having the same structure with the
// same number of nodes
// TODO: create a new struct?
func CombineTree(leftTree, rightTree *Tree) (*Tree, *Tree) {
	leftTree.Lock()
	defer leftTree.Unlock()
	rightTree.Lock()
	defer rightTree.Unlock()

	leftNodes := []*treeNode{leftTree.root}
	rghtNodes := []*treeNode{rightTree.root}

	for len(leftNodes) > 0 {
		left, rght := leftNodes[0], rghtNodes[0]
		leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]

		left.ChildrenNodes, rght.ChildrenNodes = combineNodes(left.ChildrenNodes, rght.ChildrenNodes)
		leftNodes = append(leftNodes, left.ChildrenNodes...)
		rghtNodes = append(rghtNodes, rght.ChildrenNodes...)
	}
	return leftTree, rightTree
}

func combineNodes(leftNodes, rghtNodes []*treeNode) ([]*treeNode, []*treeNode) {
	size := nextPow2(max(len(leftNodes), len(rghtNodes)))
	leftResult := make([]*treeNode, 0, size)
	rghtResult := make([]*treeNode, 0, size)

	for len(leftNodes) != 0 && len(rghtNodes) != 0 {
		left, rght := leftNodes[0], rghtNodes[0]
		switch bytes.Compare(left.Name, rght.Name) {
		case 0:
			leftResult = append(leftResult, left)
			rghtResult = append(rghtResult, rght)
			leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]
		case -1:
			leftResult = append(leftResult, left)
			rghtResult = append(rghtResult, newNode(left.Name))
			leftNodes = leftNodes[1:]
		case 1:
			leftResult = append(leftResult, newNode(rght.Name))
			rghtResult = append(rghtResult, rght)
			rghtNodes = rghtNodes[1:]
		}
	}
	leftResult = append(leftResult, leftNodes...)
	rghtResult = append(rghtResult, rghtNodes...)
	for _, left := range leftNodes {
		rghtResult = append(rghtResult, newNode(left.Name))
	}
	for _, rght := range rghtNodes {
		leftResult = append(leftResult, newNode(rght.Name))
	}
	return leftResult, rghtResult
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

	leftNodes, xLeftOffsets := []*treeNode{leftTree.root}, []int{0}
	rghtNodes, xRghtOffsets := []*treeNode{rightTree.root}, []int{0}
	levels := []int{0}
	minVal := combineMinValues(leftTree, rightTree, maxNodes)
	nameLocationCache := map[string]int{}

	for len(leftNodes) > 0 {
		left, rght := leftNodes[0], rghtNodes[0]
		leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]

		xLeftOffset, xRghtOffset := xLeftOffsets[0], xRghtOffsets[0]
		xLeftOffsets, xRghtOffsets = xLeftOffsets[1:], xRghtOffsets[1:]

		level := levels[0]
		levels = levels[1:]

		// both left.Name and rght.Name must be the same
		name := string(left.Name)
		if left.Total >= minVal || rght.Total >= minVal || name == "other" {
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
			res.MaxSelf = max(res.MaxSelf, int(rght.Self))

			// i+0 = x offset, left  tree
			// i+1 = total   , left  tree
			// i+2 = self    , left  tree
			// i+3 = x offset, right tree
			// i+4 = total   , right tree
			// i+5 = self    , right tree
			// i+6 = index in the names array
			values := []int{
				xLeftOffset, int(left.Total), int(left.Self),
				xRghtOffset, int(rght.Total), int(rght.Self),
				i,
			}
			res.Levels[level] = append(values, res.Levels[level]...)

			xLeftOffset += int(left.Self)
			xRghtOffset += int(rght.Self)
			otherLeftTotal, otherRghtTotal := uint64(0), uint64(0)

			// both left and right must have the same number of children nodes
			for ni := range left.ChildrenNodes {
				leftNode, rghtNode := left.ChildrenNodes[ni], rght.ChildrenNodes[ni]
				if leftNode.Total >= minVal || rghtNode.Total >= minVal {
					levels = prependInt(levels, level+1)
					xLeftOffsets = prependInt(xLeftOffsets, xLeftOffset)
					xRghtOffsets = prependInt(xRghtOffsets, xRghtOffset)
					leftNodes = prependTreeNode(leftNodes, leftNode)
					rghtNodes = prependTreeNode(rghtNodes, rghtNode)
					xLeftOffset += int(leftNode.Total)
					xRghtOffset += int(rghtNode.Total)
				} else {
					otherLeftTotal += leftNode.Total
					otherRghtTotal += rghtNode.Total
				}
			}
			if otherLeftTotal != 0 || otherRghtTotal != 0 {
				levels = prependInt(levels, level+1)
				{
					leftNode := &treeNode{
						Name:  jsonableSlice("other"),
						Total: otherLeftTotal,
						Self:  otherLeftTotal,
					}
					xLeftOffsets = prependInt(xLeftOffsets, xLeftOffset)
					leftNodes = prependTreeNode(leftNodes, leftNode)
				}
				{
					rghtNode := &treeNode{
						Name:  jsonableSlice("other"),
						Total: otherRghtTotal,
						Self:  otherRghtTotal,
					}
					xRghtOffsets = prependInt(xRghtOffsets, xRghtOffset)
					rghtNodes = prependTreeNode(rghtNodes, rghtNode)
				}
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
	leftNodes, rghtNodes := []*treeNode{leftTree.root}, []*treeNode{rightTree.root}
	i := 0
	for len(leftNodes) > 0 {
		leftNode, rghtNode := leftNodes[0], rghtNodes[0]
		leftNodes, rghtNodes = leftNodes[1:], rghtNodes[1:]
		i++
		if cb(leftNode.Total, rghtNode.Total) {
			leftNodes = append(leftNode.ChildrenNodes, leftNodes...)
			rghtNodes = append(rghtNode.ChildrenNodes, rghtNodes...)
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
