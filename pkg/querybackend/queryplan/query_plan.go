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
//
// maxMerges must be at least 3. Binary merge trees cannot be balanced for an
// odd number of nodes: the leftover node becomes the only child of a merge
// node, which does no useful merging.
func BuildBalanced(blocks []*metastorev1.BlockMeta, maxReads int, maxMerges int) *queryv1.QueryPlan {
	if len(blocks) == 0 || maxReads < 1 || maxMerges < 3 {
		return new(queryv1.QueryPlan)
	}

	leafNodeCount := (len(blocks) + maxReads - 1) / maxReads
	nodes := allocateContiguous[queryv1.QueryNode](leafNodeCount)

	// Uniformly assign blocks to leaf nodes.
	var start int
	for idx := range leafNodeCount {
		end := (idx + 1) * len(blocks) / leafNodeCount
		nodes[idx].Type = queryv1.QueryNode_READ
		nodes[idx].Blocks = blocks[start:end:end]
		start = end
	}

	// Recursively build a balanced tree of merge nodes.
	root := buildMergeTree(nodes, maxMerges)
	return &queryv1.QueryPlan{
		Root: root,
	}
}

// buildMergeTree will recursively create a query tree of merge nodes with at
// most maxMerges number of children. At each level, the tree will maintain a
// similar number of blocks assigned to each merge node compared to its
// siblings.
//
// The algorithm is straightforward, but it has nuances. Since len(nodes) > 1,
// we know we need to create a merge node and place nodes underneath it.
//
// When creating a merge node we want to partition the nodes evenly into groups
// such that we have no more than maxMerges groups (since a given merge node
// cannot exceed maxMerges children). Once we select a number of groups, we use
// balanceGroupItems to evenly (as evenly as possible) spread the nodes across
// all the groups.
//
// It's important to note that we want to select a groupSize which is a power of
// maxMerges. This ensures that each subtree has the same depth as its siblings.
//
// As an example, consider the following input:
//
//	maxMerges = 3
//	nodes     = [ n0 n1 n2 n3 n4 n5 n6 n7 n8 n9 ]
//
// We want to partition nodes such that each group has an even number of
// nodes itself. Naively we could compute:
//
//	groupSize = ceil(len(nodes) / maxMerges)
//
// However, this would result in an imbalanced tree:
//
//	[                   M0                  ]
//	[ n0 n1 n2 n3 ] [ n4 n5 n6 ] [ n7 n8 n9 ]
//
// From here, the first partition was given 4 nodes, which exceeds maxMerges,
// so it needs to be broken down further. The other two partitions have 3 nodes,
// so they do not need to be branched further. After splitting the first
// partition, the tree becomes imbalanced.
//
//	[                     M0                    ]
//	[        M1       ] [ n4 n5 n6 ] [ n7 n8 n9 ]
//	[ n0 n1 ] [ n2 n3 ]
//
// Instead, we select the largest power of maxMerges K that's less than
// len(nodes). In this case:
//
//	K = maxMerges^N
//	  for max(N) and maxMerges^N < len(nodes)
//	if N = 2, then 3^2 < 10 so K = 9
//
// If we allow K to represent the largest size of a partition, we can calculate
// how many partitions we need:
//
//	# of partitions = ceil(len(nodes) / K)
//	                = ceil(10 / 5)
//	                = 2
//
// Now we can balance the nodes across 2 partitions:
//
//	[                 M0                ]
//	[ n0 n1 n2 n3 n4 ] [ n5 n6 n7 n8 n9 ]
//
// Both partitions exceed maxMerges, so we repeat the algorithm:
//
//	[                     M0                    ]
//	[         M1         ] [         M2         ]
//	[ n0 n1 n2 ] [ n3 n4 ] [ n5 n6 n7 ] [ n8 n9 ]
//
// At this point, each subtree does the same amount of work as its siblings.
func buildMergeTree(nodes []*queryv1.QueryNode, maxMerges int) *queryv1.QueryNode {
	if len(nodes) == 1 {
		// The base case, this is a leaf node.
		return nodes[0]
	}

	// We have len(nodes) number of nodes. We need to partition them into groups
	// such that we have no more than maxMerges number of groups. Importantly, we
	// want a group size such that we don't place all the nodes into a single
	// group.
	groupSize := 1
	for groupSize*maxMerges < len(nodes) {
		groupSize *= maxMerges
	}

	// Given groupSize as the maximum number of nodes that can be assigned to
	// each subtree, we calculate
	//
	//	ceil(len(nodes) / groupSize)
	//
	// to determine how many children this node will have.
	childCount := (len(nodes) + groupSize - 1) / groupSize

	parent := &queryv1.QueryNode{
		Type:     queryv1.QueryNode_MERGE,
		Children: make([]*queryv1.QueryNode, childCount),
	}

	// Evenly distribute all the nodes to each child of this merge node.
	var start int
	for idx := range childCount {
		end := start + balanceGroupItems(len(nodes), childCount, idx)
		parent.Children[idx] = buildMergeTree(nodes[start:end], maxMerges)
		start = end
	}

	return parent
}

// balanceGroupItems will take numItems and distribute them evenly across
// numGroups groups. If there is a remainder R, that remainder is then spread
// evenly across the first R groups. It returns the number of items groupIdx
// should have for all groups to remain balanced.
//
// For example, given these parameters:
//
//	numItems  = 5
//	numGroups = 3
//
// We would get the following (for various values of groupIdx):
//
//	Group 0: 2
//	Group 1: 2
//	Group 2: 1
func balanceGroupItems(numItems int, numGroups int, groupIdx int) int {
	groupSize := numItems / numGroups
	if groupIdx < numItems%numGroups {
		groupSize++
	}
	return groupSize
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
