package tree

import "sort"

// minValue returns kth-smallest node total value.
func (t *Tree) minValue(maxNodes int) uint64 {
	if len(t.nodes) <= maxNodes {
		return 0
	}
	nodes := make([]uint64, len(t.nodes))
	for i := range t.nodes {
		nodes[i] = t.nodes[i].Total
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[j] < nodes[i]
	})
	return nodes[maxNodes]
}
