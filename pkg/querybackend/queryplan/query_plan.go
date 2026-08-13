package queryplan

import (
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// Build creates a query plan from the list of block metadata.
//
// NOTE(kolesnikovae): At this point it only groups blocks into uniform ranges,
// and builds a DAG of reads and merges. In practice, however, we may want to
// implement more sophisticated strategies. For example, it would be beneficial
// to group blocks based on the tenant services to ensure that a single read
// covers exactly one service, and does not have to deal with stack trace
// cardinality issues. Another example is grouping by shards to minimize the
// number of unique series (assuming the shards are still built based on the
// series labels) a reader or merger should handle. In general, the strategy
// should depend on the query type.
func Build(
	blocks []*metastorev1.BlockMeta,
	maxReads, maxMerges int,
) *queryv1.QueryPlan {
	if len(blocks) == 0 {
		return new(queryv1.QueryPlan)
	}

	if maxReads < 1 {
		return new(queryv1.QueryPlan)
	}

	if maxMerges < 2 {
		return new(queryv1.QueryPlan)
	}

	// create leaf nodes and spread the blocks in a uniform way
	leafNodeCount := (len(blocks) + maxReads - 1) / maxReads
	nodes := allocateContiguous[queryv1.QueryNode](leafNodeCount)
	for start, idx := 0, 0; start < len(blocks); start, idx = start+maxReads, idx+1 {
		end := min(start+maxReads, len(blocks))
		nodes[idx].Type = queryv1.QueryNode_READ
		nodes[idx].Blocks = blocks[start:end]
	}

	// create merge nodes until we reach a single root node
	for len(nodes) > 1 {
		mergeNodeCount := (len(nodes) + maxMerges - 1) / maxMerges
		mergeNodes := allocateContiguous[queryv1.QueryNode](mergeNodeCount)

		for start, idx := 0, 0; start < len(nodes); start, idx = start+maxMerges, idx+1 {
			end := min(start+maxMerges, len(nodes))
			mergeNodes[idx].Type = queryv1.QueryNode_MERGE
			mergeNodes[idx].Children = nodes[start:end:end]
		}

		nodes = mergeNodes
	}

	return &queryv1.QueryPlan{
		Root: nodes[0],
	}
}

// BuildBalanced builds a balanced query tree where each node of the tree has a
// similar number of blocks it needs to process compared to its siblings.
func BuildBalanced(blocks []*metastorev1.BlockMeta, maxReads int, maxMerges int) *queryv1.QueryPlan {
	if len(blocks) == 0 || maxReads < 1 || maxMerges < 2 {
		return new(queryv1.QueryPlan)
	}

	// Spread the blocks over the smallest possible number of read nodes. The
	// boundaries are derived from the block index rather than accumulated, so
	// that any contiguous run of read nodes covers a proportional share of the
	// blocks. That is what lets mergeTree group nodes by count and still come
	// out balanced by block count.
	readNodeCount := (len(blocks) + maxReads - 1) / maxReads
	nodes := allocateContiguous[queryv1.QueryNode](readNodeCount)
	for idx, start := 0, 0; idx < readNodeCount; idx++ {
		end := (idx + 1) * len(blocks) / readNodeCount
		nodes[idx].Type = queryv1.QueryNode_READ
		nodes[idx].Blocks = blocks[start:end:end]
		start = end
	}

	return &queryv1.QueryPlan{
		Root: mergeTree(nodes, maxMerges),
	}
}

// mergeTree groups nodes under merge nodes of at most maxMerges children, and
// recurses until a single root node is left. Sibling groups differ in size by
// at most one node, and a merge node always has at least two children: a group
// of one is linked directly to its parent instead of being wrapped in a merge
// node that would do nothing but relay it.
func mergeTree(nodes []*queryv1.QueryNode, maxMerges int) *queryv1.QueryNode {
	if len(nodes) == 1 {
		return nodes[0]
	}

	// Every merge level multiplies the number of nodes a subtree can cover by
	// maxMerges. Take the shallowest subtree that lets maxMerges children cover
	// all of them.
	capacity := 1
	for capacity*maxMerges < len(nodes) {
		capacity *= maxMerges
	}

	// Invariant: 2 <= childCount <= maxMerges.
	childCount := (len(nodes) + capacity - 1) / capacity
	parent := &queryv1.QueryNode{
		Type:     queryv1.QueryNode_MERGE,
		Children: make([]*queryv1.QueryNode, childCount),
	}
	for idx, start := 0, 0; idx < childCount; idx++ {
		end := start + balancedGroupSize(len(nodes), childCount, idx)
		parent.Children[idx] = mergeTree(nodes[start:end], maxMerges)
		start = end
	}

	return parent
}

// balancedGroupSize will take a totalSize and distribute it evenly across
// groupCount groups. If there is a remainder R, that remainder is then spread
// evenly across the first R groups.
//
// For example, given these parameters:
//
//	totalSize  = 5
//	groupCount = 3
//
// We would get the following (for various values of groupIdx):
//
//	Group 0: 2
//	Group 1: 2
//	Group 2: 1
func balancedGroupSize(totalSize int, groupCount int, groupIdx int) int {
	size := totalSize / groupCount
	if groupIdx < totalSize%groupCount {
		size++
	}
	return size
}

// allocateContiguous returns a []*T of length size where every element points
// into a single backing []T allocation. This avoids the per-element heap
// allocations from N separate &T{} expressions.
func allocateContiguous[T any](size int) []*T {
	values := make([]T, size)
	pointers := make([]*T, size)
	for i := range values {
		pointers[i] = &values[i]
	}
	return pointers
}
