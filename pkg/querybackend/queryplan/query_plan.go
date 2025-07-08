package queryplan

import (
	"fmt"
	"io"
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
	var leafNodes []*queryv1.QueryNode
	for i := 0; i < len(blocks); i += maxReads {
		end := i + maxReads
		if end > len(blocks) {
			end = len(blocks)
		}
		leafNodes = append(leafNodes, &queryv1.QueryNode{
			Type:   queryv1.QueryNode_READ,
			Blocks: blocks[i:end],
		})
	}

	// create merge nodes until we reach a single root node
	for len(leafNodes) > 1 {
		numNodes := len(leafNodes)
		numMerges := int(math.Ceil(float64(numNodes) / float64(maxMerges)))

		var newLeafNodes []*queryv1.QueryNode
		for i := 0; i < numMerges; i++ {
			newNode := &queryv1.QueryNode{
				Type: queryv1.QueryNode_MERGE,
			}
			for j := 0; j < maxMerges && len(leafNodes) > 0; j++ {
				newNode.Children = append(newNode.Children, leafNodes[0])
				leafNodes = leafNodes[1:]
			}
			newLeafNodes = append(newLeafNodes, newNode)
		}
		leafNodes = newLeafNodes
	}

	return &queryv1.QueryPlan{
		Root: leafNodes[0],
	}
}

func printPlan(w io.Writer, pad string, n *queryv1.QueryNode, debug bool) {
	if debug {
		_, _ = fmt.Fprintf(w, pad+"%s {children: %d, blocks: %d}\n",
			n.Type, len(n.Children), len(n.Blocks))
	} else {
		_, _ = fmt.Fprintf(w, pad+"%s (%d)\n", n.Type, len(n.Children))
	}

	switch n.Type {
	case queryv1.QueryNode_MERGE:
		for _, child := range n.Children {
			printPlan(w, pad+"\t", child, debug)
		}

	case queryv1.QueryNode_READ:
		for _, md := range n.Blocks {
			_, _ = fmt.Fprintf(w, pad+"\t"+"id:\"%s\"\n", md.Id)
		}

	default:
		panic("unknown type")
	}
}
