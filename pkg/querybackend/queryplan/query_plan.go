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
	var nodes []*queryv1.QueryNode
	for i := 0; i < len(blocks); i += maxReads {
		end := i + maxReads
		if end > len(blocks) {
			end = len(blocks)
		}
		nodes = append(nodes, &queryv1.QueryNode{
			Type:   queryv1.QueryNode_READ,
			Blocks: blocks[i:end],
		})
	}

	// create merge nodes until we reach a single root node
	for len(nodes) > 1 {
		mergeNodeCount := (len(nodes) + maxMerges - 1) / maxMerges
		mergeNodes := make([]*queryv1.QueryNode, 0, mergeNodeCount)

		for i := 0; i < len(nodes); i += maxMerges {
			end := i + maxMerges
			if end > len(nodes) {
				end = len(nodes)
			}

			mergeNodes = append(mergeNodes, &queryv1.QueryNode{
				Type:     queryv1.QueryNode_MERGE,
				Children: nodes[i:end:end],
			})
		}

		nodes = mergeNodes
	}

	return &queryv1.QueryPlan{
		Root: nodes[0],
	}
}
