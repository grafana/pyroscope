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
		end := start + maxReads
		if end > len(blocks) {
			end = len(blocks)
		}
		nodes[idx].Type = queryv1.QueryNode_READ
		nodes[idx].Blocks = blocks[start:end]
	}

	// create merge nodes until we reach a single root node
	for len(nodes) > 1 {
		mergeNodeCount := (len(nodes) + maxMerges - 1) / maxMerges
		mergeNodes := allocateContiguous[queryv1.QueryNode](mergeNodeCount)

		for start, idx := 0, 0; start < len(nodes); start, idx = start+maxMerges, idx+1 {
			end := start + maxMerges
			if end > len(nodes) {
				end = len(nodes)
			}
			mergeNodes[idx].Type = queryv1.QueryNode_MERGE
			mergeNodes[idx].Children = nodes[start:end:end]
		}

		nodes = mergeNodes
	}

	return &queryv1.QueryPlan{
		Root: nodes[0],
	}
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
