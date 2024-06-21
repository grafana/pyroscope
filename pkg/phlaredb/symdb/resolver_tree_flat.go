package symdb

import (
	"container/heap"

	"github.com/grafana/pyroscope/pkg/model"
)

func buildTree(
	tree ParentPointerTree,
	values SampleValues,
	symbols *Symbols,
	maxNodes int64,
) *model.Tree {
	nodes := tree.Nodes()
	values.SetNodeValues(nodes)
	propagateNodeValues(nodes)
	markNodes(nodes, maxNodes*2)
	t := model.NewStacktraceTree(int(maxNodes))
	l := int32(len(nodes))
	s := make([]int32, 0, 64)
	for i := int32(1); i < l; i++ {
		p := nodes[i].Parent
		v := nodes[i].Value
		if v > 0 && nodes[p].Location&truncationMark == 0 {
			s = resolveStack(s, nodes, i, symbols)
			t.Insert(s, v)
		}
	}
	return t.Tree(maxNodes, symbols.Strings)
}

func propagateNodeValues(nodes []Node) {
	// Step 1: Set leaf values.
	// This is already done by the caller.
	// Presence of the value does not indicate
	// that the node is a leaf: it may have its
	// own value and children.

	// Step 2: Propagate values to the direct
	// parent. We iterate the nodes in reverse
	// order to ensure that all the descendants
	// have been processed before the parent,
	// and its value includes all the children.
	// TODO: This step could be repeated multiple times.
	for i := len(nodes) - 1; i >= 1; i-- {
		if p := nodes[i].Parent; p > 0 {
			nodes[p].Value += nodes[i].Value
		}
	}
	// Step 3: Find edge nodes: ones that have
	// children, but do not have a value.
	// Sum up the values of the children and
	// mark the node – their values are to
	// be propagated to the parent chain at
	// the next step.
	const mark = 1 << 30
	for i := len(nodes) - 1; i >= 1; i-- {
		if p := nodes[i].Parent; p > 0 && nodes[p].Value == 0 {
			nodes[p].Value += nodes[i].Value
			nodes[p].Location |= mark
		}
	}
	// Step 4: Propagate the edge node values
	// to the parent chain. Propagation stops
	// once we find another edge node in the
	// chain: we add own value to it, which will
	// be propagated further, after all the
	// downstream edges converge.
	for i := len(nodes) - 1; i >= 1; i-- {
		if nodes[i].Location&mark > 0 {
			nodes[i].Location &= ^mark
			v := nodes[i].Value
			j := nodes[i].Parent
			for j >= 0 {
				nodes[j].Value += v
				if nodes[j].Location&mark != 0 {
					break
				}
				j = nodes[j].Parent
			}
		}
	}
}

func markNodes(dst []Node, maxNodes int64) {
	m := minValue(dst, maxNodes)
	for i := 1; i < len(dst); i++ {
		if dst[i].Value < m {
			dst[i].Location |= truncationMark
			// Preserve value of the truncated location on leaves.
			if dst[dst[i].Parent].Location&truncationMark != 0 {
				continue
			}
		}
		dst[dst[i].Parent].Value -= dst[i].Value
	}
}

func resolveStack(dst []int32, nodes []Node, i int32, symbols *Symbols) []int32 {
	dst = dst[:0]
	for i > 0 {
		j := nodes[i].Location
		if j&truncationMark > 0 && len(dst) == 0 {
			dst = append(dst, sentinel)
		} else {
			loc := symbols.Locations[j]
			for l := 0; l < len(loc.Line); l++ {
				dst = append(dst, int32(symbols.Functions[loc.Line[l].FunctionId].Name))
			}
		}
		i = nodes[i].Parent
	}
	return dst
}

func minValue(nodes []Node, maxNodes int64) int64 {
	if maxNodes < 1 || maxNodes >= int64(len(nodes)) {
		return 0
	}
	s := make(minHeap, 0, maxNodes)
	h := &s
	for i := range nodes {
		v := nodes[i].Value
		if h.Len() >= int(maxNodes) {
			if v > (*h)[0] {
				heap.Pop(h)
			} else {
				continue
			}
		}
		heap.Push(h, v)
	}
	if h.Len() < int(maxNodes) {
		return 0
	}
	return (*h)[0]
}

type minHeap []int64

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i] < h[j] }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x interface{}) { *h = append(*h, x.(int64)) }

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
