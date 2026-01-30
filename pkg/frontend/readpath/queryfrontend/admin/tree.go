package admin

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func buildMetadataStats(blocks []*metastorev1.BlockMeta, startTime, endTime time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Blocks found: %d\n", len(blocks))
	fmt.Fprintf(&sb, "Time range: %s to %s", startTime.UTC().Format(time.RFC3339), endTime.UTC().Format(time.RFC3339))

	if len(blocks) == 0 {
		return sb.String()
	}

	var totalBlockSize uint64
	var totalDatasetSize uint64
	var totalDatasets int

	var largestBlock, smallestBlock *metastorev1.BlockMeta
	var largestDataset, smallestDataset *metastorev1.Dataset
	var largestDatasetBlock, smallestDatasetBlock *metastorev1.BlockMeta

	for _, block := range blocks {
		totalBlockSize += block.Size

		if largestBlock == nil || block.Size > largestBlock.Size {
			largestBlock = block
		}
		if smallestBlock == nil || block.Size < smallestBlock.Size {
			smallestBlock = block
		}

		for _, ds := range block.Datasets {
			totalDatasetSize += ds.Size
			totalDatasets++

			if largestDataset == nil || ds.Size > largestDataset.Size {
				largestDataset = ds
				largestDatasetBlock = block
			}
			if smallestDataset == nil || ds.Size < smallestDataset.Size {
				smallestDataset = ds
				smallestDatasetBlock = block
			}
		}
	}

	// Block statistics
	fmt.Fprintf(&sb, "\n\nBlock Statistics:\n")
	fmt.Fprintf(&sb, "  Total size: %s\n", humanize.Bytes(totalBlockSize))
	if len(blocks) > 0 {
		avgBlockSize := totalBlockSize / uint64(len(blocks))
		fmt.Fprintf(&sb, "  Average size: %s\n", humanize.Bytes(avgBlockSize))
	}
	if largestBlock != nil {
		fmt.Fprintf(&sb, "  Largest: %s (%s, shard %d, L%d)\n", humanize.Bytes(largestBlock.Size), largestBlock.Id, largestBlock.Shard, largestBlock.CompactionLevel)
	}
	if smallestBlock != nil {
		fmt.Fprintf(&sb, "  Smallest: %s (%s, shard %d, L%d)", humanize.Bytes(smallestBlock.Size), smallestBlock.Id, smallestBlock.Shard, smallestBlock.CompactionLevel)
	}

	// Dataset statistics
	if totalDatasets > 0 {
		fmt.Fprintf(&sb, "\n\nDataset Statistics:\n")
		fmt.Fprintf(&sb, "  Total datasets: %d\n", totalDatasets)
		fmt.Fprintf(&sb, "  Total size: %s\n", humanize.Bytes(totalDatasetSize))
		avgDatasetSize := totalDatasetSize / uint64(totalDatasets)
		fmt.Fprintf(&sb, "  Average size: %s\n", humanize.Bytes(avgDatasetSize))
		if largestDataset != nil && largestDatasetBlock != nil {
			dsName := getDatasetName(largestDataset, largestDatasetBlock)
			fmt.Fprintf(&sb, "  Largest: %s (%s in %s, shard %d, L%d)\n", humanize.Bytes(largestDataset.Size), dsName, largestDatasetBlock.Id, largestDatasetBlock.Shard, largestDatasetBlock.CompactionLevel)
		}
		if smallestDataset != nil && smallestDatasetBlock != nil {
			dsName := getDatasetName(smallestDataset, smallestDatasetBlock)
			fmt.Fprintf(&sb, "  Smallest: %s (%s in %s, shard %d, L%d)", humanize.Bytes(smallestDataset.Size), dsName, smallestDatasetBlock.Id, smallestDatasetBlock.Shard, smallestDatasetBlock.CompactionLevel)
		}
	}

	return sb.String()
}

func getDatasetName(ds *metastorev1.Dataset, block *metastorev1.BlockMeta) string {
	if ds.Name >= 0 && int(ds.Name) < len(block.StringTable) {
		return block.StringTable[ds.Name]
	}
	return fmt.Sprintf("dataset-%d", ds.Name)
}

func extractBlocksFromPlan(plan *queryv1.QueryPlan) []*metastorev1.BlockMeta {
	if plan == nil || plan.Root == nil {
		return nil
	}
	var blocks []*metastorev1.BlockMeta
	extractBlocksFromNode(plan.Root, &blocks)
	return blocks
}

func extractBlocksFromNode(node *queryv1.QueryNode, blocks *[]*metastorev1.BlockMeta) {
	if node == nil {
		return
	}
	if node.Type == queryv1.QueryNode_READ {
		*blocks = append(*blocks, node.Blocks...)
	}
	for _, child := range node.Children {
		extractBlocksFromNode(child, blocks)
	}
}

func convertQueryPlanToTree(plan *queryv1.QueryPlan) *PlanTreeNode {
	if plan == nil || plan.Root == nil {
		return nil
	}
	return convertQueryNodeToTree(plan.Root)
}

func convertQueryNodeToTree(node *queryv1.QueryNode) *PlanTreeNode {
	if node == nil {
		return nil
	}

	treeNode := &PlanTreeNode{}

	switch node.Type {
	case queryv1.QueryNode_MERGE:
		treeNode.Type = "MERGE"
		treeNode.Children = make([]*PlanTreeNode, 0, len(node.Children))
		for _, child := range node.Children {
			childNode := convertQueryNodeToTree(child)
			if childNode != nil {
				treeNode.Children = append(treeNode.Children, childNode)
				treeNode.TotalBlocks += childNode.TotalBlocks
			}
		}
	case queryv1.QueryNode_READ:
		treeNode.Type = "READ"
		treeNode.BlockCount = len(node.Blocks)
		treeNode.TotalBlocks = len(node.Blocks)
		for _, block := range node.Blocks {
			treeNode.Blocks = append(treeNode.Blocks, PlanTreeBlock{
				ID:              block.Id,
				Shard:           block.Shard,
				Size:            humanize.Bytes(block.Size),
				CompactionLevel: block.CompactionLevel,
			})
		}
	default:
		treeNode.Type = "UNKNOWN"
	}

	return treeNode
}

func convertExecutionNodeToTree(node *queryv1.ExecutionNode) *ExecutionTreeNode {
	if node == nil {
		return nil
	}

	// Find the earliest start time across all nodes to use as query start reference
	queryStartNs := findEarliestStartTime(node)

	return convertExecutionNodeToTreeWithBase(node, queryStartNs)
}

func findEarliestStartTime(node *queryv1.ExecutionNode) int64 {
	if node == nil {
		return 0
	}

	earliest := node.StartTimeNs

	// Check block executions
	if node.Stats != nil {
		for _, blockExec := range node.Stats.BlockExecutions {
			if blockExec.StartTimeNs < earliest {
				earliest = blockExec.StartTimeNs
			}
		}
	}

	// Check children
	for _, child := range node.Children {
		childEarliest := findEarliestStartTime(child)
		if childEarliest > 0 && childEarliest < earliest {
			earliest = childEarliest
		}
	}

	return earliest
}

func convertExecutionNodeToTreeWithBase(node *queryv1.ExecutionNode, queryStartNs int64) *ExecutionTreeNode {
	if node == nil {
		return nil
	}

	duration := time.Duration(node.EndTimeNs - node.StartTimeNs)
	relativeStart := time.Duration(node.StartTimeNs - queryStartNs)

	tree := &ExecutionTreeNode{
		Type:             node.Type.String(),
		Executor:         node.Executor,
		Duration:         duration,
		DurationStr:      formatDurationShort(duration),
		RelativeStart:    relativeStart,
		RelativeStartStr: formatDurationShort(relativeStart),
		Error:            node.Error,
	}

	if node.Stats != nil {
		tree.Stats = &ExecutionTreeStats{
			BlocksRead:        node.Stats.BlocksRead,
			DatasetsProcessed: node.Stats.DatasetsProcessed,
		}

		for _, blockExec := range node.Stats.BlockExecutions {
			blockDuration := time.Duration(blockExec.EndTimeNs - blockExec.StartTimeNs)
			blockRelStart := time.Duration(blockExec.StartTimeNs - queryStartNs)
			blockRelEnd := time.Duration(blockExec.EndTimeNs - queryStartNs)

			tree.Stats.BlockExecutions = append(tree.Stats.BlockExecutions, &BlockExecutionInfo{
				BlockID:           blockExec.BlockId,
				Duration:          blockDuration,
				DurationStr:       formatDurationShort(blockDuration),
				RelativeStart:     blockRelStart,
				RelativeStartStr:  formatDurationShort(blockRelStart),
				RelativeEnd:       blockRelEnd,
				RelativeEndStr:    formatDurationShort(blockRelEnd),
				DatasetsProcessed: blockExec.DatasetsProcessed,
				Size:              humanize.Bytes(blockExec.Size),
				Shard:             blockExec.Shard,
				CompactionLevel:   blockExec.CompactionLevel,
			})
		}
	}

	for _, child := range node.Children {
		if childTree := convertExecutionNodeToTreeWithBase(child, queryStartNs); childTree != nil {
			tree.Children = append(tree.Children, childTree)
		}
	}

	return tree
}
