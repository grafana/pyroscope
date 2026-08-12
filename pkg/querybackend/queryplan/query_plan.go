package queryplan

import (
	"math"

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

func BuildBalanced(blocks []*metastorev1.BlockMeta, maxReads int, maxMerges int) *queryv1.QueryPlan {
	if len(blocks) == 0 || maxReads < 1 || maxMerges < 2 {
		return new(queryv1.QueryPlan)
	}

	readNodeCount := int(math.Ceil(float64(len(blocks)) / float64(maxReads)))
	nodes := allocateContiguous[queryv1.QueryNode](readNodeCount)
	weights := make([]int, readNodeCount)

	// Build the read nodes, balancing blocks across all read nodes. We also
	// record the number of blocks in each read node to be used later when
	// assigning read nodes to merge nodes.
	for start, idx := 0, 0; idx < readNodeCount; idx++ {
		size := balancedGroupSize(len(blocks), readNodeCount, idx)
		end := start + size

		nodes[idx].Type = queryv1.QueryNode_READ
		nodes[idx].Blocks = blocks[start:end:end]
		weights[idx] = size
		start = end
	}

	// Build the merge nodes. We assign merge children to merge nodes based on the
	// number of blocks being merged at each level. We want each merge node to
	// merge approximately the same number of blocks as its siblings.
	for len(nodes) > 1 {
		mergeNodeCount := int(math.Ceil(float64(len(nodes)) / float64(maxMerges)))
		mergeNodes := allocateContiguous[queryv1.QueryNode](mergeNodeCount)
		mergeWeights := make([]int, mergeNodeCount)

		// workloadGroupSizes calculates how many children each merge node should
		// have at this level.
		groupSizes := distributeChildNodes(weights, mergeNodeCount, maxMerges)
		start := 0
		for idx, size := range groupSizes {
			end := start + size
			mergeNodes[idx].Type = queryv1.QueryNode_MERGE
			mergeNodes[idx].Children = nodes[start:end:end]

			for _, weight := range weights[start:end] {
				mergeWeights[idx] += weight
			}
			start = end
		}

		nodes = mergeNodes
		weights = mergeWeights
	}

	return &queryv1.QueryPlan{
		Root: nodes[0],
	}
}

func distributeChildNodes(childNodeBlocks []int, mergeNodeCount int, maxMergeNodeSize int) []int {
	// This slice contains the number of child nodes each merge node should be
	// allocated.
	mergeNodeChildren := make([]int, mergeNodeCount)

	remainingTotalBlockCount := 0
	for _, count := range childNodeBlocks {
		remainingTotalBlockCount += count
	}

	currentChildNodeIdx := 0
	for mergeNodeIdx := range mergeNodeCount {
		// Calculate the remaining merge nodes and child nodes we have left to
		// allocate.
		remainingMergeNodes := mergeNodeCount - mergeNodeIdx
		remainingChildNodes := len(childNodeBlocks) - currentChildNodeIdx

		// Calculate the minimum and maximum child nodes this merge node can have.
		minChildNodeCount := max(1, remainingChildNodes-(remainingMergeNodes-1)*maxMergeNodeSize)
		maxChildNodeCount := min(maxMergeNodeSize, remainingChildNodes-(remainingMergeNodes-1))

		// Calculate the ideal number of blocks we could give to this merge node
		// (and all subsequent merge nodes) to evenly distribute blocks amongst
		// them. This number likely will not be a whole number.
		targetBlockCount := float64(remainingTotalBlockCount) / float64(remainingMergeNodes)

		bestChildNodeCount := minChildNodeCount
		bestBlockCount := 0
		bestDistance := float64(0)

		// We have to allocate at least the minimum number of child nodes to this
		// merge node, so we do that now. we then calcualte how far away we are from
		// the ideal allocation.
		for _, b := range childNodeBlocks[currentChildNodeIdx : currentChildNodeIdx+minChildNodeCount] {
			bestBlockCount += b
		}
		bestDistance = math.Abs(float64(bestBlockCount) - targetBlockCount)

		// Now we expand the number of child nodes we give to this merge node to see
		// if we can get closer to the ideal block count.
		candidateBlockCount := bestBlockCount
		for nodeCount := minChildNodeCount + 1; nodeCount <= maxChildNodeCount; nodeCount++ {
			candidateBlockCount += childNodeBlocks[currentChildNodeIdx+nodeCount-1]
			distance := math.Abs(float64(candidateBlockCount) - targetBlockCount)
			if distance < bestDistance {
				bestChildNodeCount = nodeCount
				bestBlockCount = candidateBlockCount
				bestDistance = distance
			}

			if float64(candidateBlockCount) >= targetBlockCount {
				// We are past the ideal block count, adding more child nodes won't get
				// us closer to the ideal distance.
				break
			}
		}

		mergeNodeChildren[mergeNodeIdx] = bestChildNodeCount
		currentChildNodeIdx += bestChildNodeCount
		remainingTotalBlockCount -= bestBlockCount
	}

	return mergeNodeChildren
}

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
