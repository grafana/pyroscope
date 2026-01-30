import type {
  BlockExecutionInfo,
  BlockMeta,
  ExecutionTreeNode,
  ExecutionTreeStats,
  PlanTreeNode,
  RawBlockMeta,
  RawExecutionNode,
  RawQueryNode,
  RawQueryPlan,
} from './types';

export function getBasePath(): string {
  const path = window.location.pathname;
  const match = path.match(/^(.*\/query-frontend)\//);
  if (match) {
    return match[1];
  }
  return '';
}

// Protobuf enums are serialized as numbers by Go's json.Marshal
// QueryNode_UNKNOWN = 0, QueryNode_MERGE = 1, QueryNode_READ = 2
const QUERY_NODE_TYPE_MAP: Record<number | string, 'MERGE' | 'READ'> = {
  1: 'MERGE',
  2: 'READ',
  MERGE: 'MERGE',
  READ: 'READ',
};

function getNodeType(type: number | string): 'MERGE' | 'READ' {
  return QUERY_NODE_TYPE_MAP[type] || 'MERGE';
}

export function formatDuration(ns: number): string {
  const ms = ns / 1e6;
  if (ms < 1) {
    return `${ms.toFixed(3)}ms`;
  }
  if (ms < 1000) {
    return `${ms.toFixed(3)}ms`;
  }
  return `${(ms / 1000).toFixed(3)}s`;
}

export function formatMs(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  return `${(ms / 1000).toFixed(2)}s`;
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '-';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let unitIndex = 0;
  let size = bytes;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${size.toFixed(1)} ${units[unitIndex]}`;
}

export function formatTime(isoString: string): string {
  const d = new Date(isoString);
  return d.toISOString().replace('T', ' ').substring(0, 19);
}

export function convertQueryPlanToTree(plan: RawQueryPlan): PlanTreeNode | null {
  if (!plan || !plan.root) {
    return null;
  }
  return convertQueryNodeToTree(plan.root);
}

function convertQueryNodeToTree(node: RawQueryNode): PlanTreeNode | null {
  if (!node) {
    return null;
  }

  const nodeType = getNodeType(node.type);

  const treeNode: PlanTreeNode = {
    type: nodeType,
    children: [],
    blockCount: 0,
    blocks: [],
    totalBlocks: 0,
  };

  if (nodeType === 'MERGE') {
    treeNode.children = (node.children || [])
      .map(convertQueryNodeToTree)
      .filter((n): n is PlanTreeNode => n !== null);
    treeNode.totalBlocks = treeNode.children.reduce(
      (sum, child) => sum + child.totalBlocks,
      0
    );
  } else {
    treeNode.blockCount = node.blocks?.length || 0;
    treeNode.totalBlocks = treeNode.blockCount;
    treeNode.blocks = (node.blocks || []).map((block: RawBlockMeta) => ({
      id: block.id,
      shard: block.shard ?? 0,
      size: formatBytes(block.size),
      compactionLevel: block.compaction_level ?? 0,
    }));
  }

  return treeNode;
}

export function extractBlocksFromPlan(plan: RawQueryPlan): BlockMeta[] {
  if (!plan || !plan.root) {
    return [];
  }
  const blocks: BlockMeta[] = [];
  extractBlocksFromNode(plan.root, blocks);
  return blocks;
}

function extractBlocksFromNode(node: RawQueryNode, blocks: BlockMeta[]): void {
  if (!node) return;
  const nodeType = getNodeType(node.type);
  if (nodeType === 'READ' && node.blocks) {
    for (const rawBlock of node.blocks) {
      blocks.push({
        id: rawBlock.id,
        shard: rawBlock.shard,
        size: rawBlock.size,
        compaction_level: rawBlock.compaction_level,
        datasets: rawBlock.datasets,
        string_table: rawBlock.string_table,
      });
    }
  }
  for (const child of node.children || []) {
    extractBlocksFromNode(child, blocks);
  }
}

export function buildMetadataStats(
  blocks: BlockMeta[],
  startTime: Date,
  endTime: Date
): string {
  let result = `Blocks found: ${blocks.length}\n`;
  result += `Time range: ${startTime.toISOString()} to ${endTime.toISOString()}`;

  if (blocks.length === 0) {
    return result;
  }

  let totalBlockSize = 0;
  let totalDatasetSize = 0;
  let totalDatasets = 0;

  let largestBlock: BlockMeta | null = null;
  let smallestBlock: BlockMeta | null = null;

  interface DatasetInfo {
    dataset: { name: number; size: number };
    block: BlockMeta;
  }
  let largestDataset: DatasetInfo | null = null;
  let smallestDataset: DatasetInfo | null = null;

  for (const block of blocks) {
    totalBlockSize += block.size;

    if (!largestBlock || block.size > largestBlock.size) {
      largestBlock = block;
    }
    if (!smallestBlock || block.size < smallestBlock.size) {
      smallestBlock = block;
    }

    for (const ds of block.datasets || []) {
      totalDatasetSize += ds.size;
      totalDatasets++;

      if (!largestDataset || ds.size > largestDataset.dataset.size) {
        largestDataset = { dataset: ds, block };
      }
      if (!smallestDataset || ds.size < smallestDataset.dataset.size) {
        smallestDataset = { dataset: ds, block };
      }
    }
  }

  result += '\n\nBlock Statistics:\n';
  result += `  Total size: ${formatBytes(totalBlockSize)}\n`;
  if (blocks.length > 0) {
    const avgBlockSize = Math.floor(totalBlockSize / blocks.length);
    result += `  Average size: ${formatBytes(avgBlockSize)}\n`;
  }
  if (largestBlock) {
    result += `  Largest: ${formatBytes(largestBlock.size)} (${largestBlock.id}, shard ${largestBlock.shard}, L${largestBlock.compaction_level ?? 0})\n`;
  }
  if (smallestBlock) {
    result += `  Smallest: ${formatBytes(smallestBlock.size)} (${smallestBlock.id}, shard ${smallestBlock.shard}, L${smallestBlock.compaction_level ?? 0})`;
  }

  if (totalDatasets > 0) {
    result += '\n\nDataset Statistics:\n';
    result += `  Total datasets: ${totalDatasets}\n`;
    result += `  Total size: ${formatBytes(totalDatasetSize)}\n`;
    const avgDatasetSize = Math.floor(totalDatasetSize / totalDatasets);
    result += `  Average size: ${formatBytes(avgDatasetSize)}\n`;
    if (largestDataset) {
      const dsName = getDatasetName(largestDataset.dataset, largestDataset.block);
      result += `  Largest: ${formatBytes(largestDataset.dataset.size)} (${dsName} in ${largestDataset.block.id}, shard ${largestDataset.block.shard}, L${largestDataset.block.compaction_level ?? 0})\n`;
    }
    if (smallestDataset) {
      const dsName = getDatasetName(smallestDataset.dataset, smallestDataset.block);
      result += `  Smallest: ${formatBytes(smallestDataset.dataset.size)} (${dsName} in ${smallestDataset.block.id}, shard ${smallestDataset.block.shard}, L${smallestDataset.block.compaction_level ?? 0})`;
    }
  }

  return result;
}

function getDatasetName(
  ds: { name: number; size: number },
  block: BlockMeta
): string {
  if (
    ds.name >= 0 &&
    block.string_table &&
    ds.name < block.string_table.length
  ) {
    return block.string_table[ds.name];
  }
  return `dataset-${ds.name}`;
}

export function convertExecutionNodeToTree(
  node: RawExecutionNode
): ExecutionTreeNode | null {
  if (!node) {
    return null;
  }

  const queryStartNs = findEarliestStartTime(node);
  return convertExecutionNodeToTreeWithBase(node, queryStartNs);
}

function findEarliestStartTime(node: RawExecutionNode): number {
  if (!node) return 0;

  let earliest = node.start_time_ns;

  if (node.stats?.block_executions) {
    for (const blockExec of node.stats.block_executions) {
      if (blockExec.start_time_ns < earliest) {
        earliest = blockExec.start_time_ns;
      }
    }
  }

  for (const child of node.children || []) {
    const childEarliest = findEarliestStartTime(child);
    if (childEarliest > 0 && childEarliest < earliest) {
      earliest = childEarliest;
    }
  }

  return earliest;
}

function convertExecutionNodeToTreeWithBase(
  node: RawExecutionNode,
  queryStartNs: number
): ExecutionTreeNode | null {
  if (!node) {
    return null;
  }

  const durationNs = node.end_time_ns - node.start_time_ns;
  const relativeStartNs = node.start_time_ns - queryStartNs;

  const nodeType = getNodeType(node.type);

  const tree: ExecutionTreeNode = {
    type: nodeType,
    executor: node.executor,
    duration: durationNs,
    durationStr: formatDuration(durationNs),
    relativeStart: relativeStartNs,
    relativeStartStr: formatDuration(relativeStartNs),
    error: node.error,
  };

  if (node.stats) {
    const stats: ExecutionTreeStats = {
      blocksRead: node.stats.blocks_read,
      datasetsProcessed: node.stats.datasets_processed,
      blockExecutions: [],
    };

    for (const blockExec of node.stats.block_executions || []) {
      const blockDurationNs = blockExec.end_time_ns - blockExec.start_time_ns;
      const blockRelStartNs = blockExec.start_time_ns - queryStartNs;
      const blockRelEndNs = blockExec.end_time_ns - queryStartNs;

      const blockInfo: BlockExecutionInfo = {
        blockId: blockExec.block_id,
        duration: blockDurationNs,
        durationStr: formatDuration(blockDurationNs),
        relativeStart: blockRelStartNs,
        relativeStartStr: formatDuration(blockRelStartNs),
        relativeEnd: blockRelEndNs,
        relativeEndStr: formatDuration(blockRelEndNs),
        datasetsProcessed: blockExec.datasets_processed ?? 0,
        size: formatBytes(blockExec.size),
        shard: blockExec.shard ?? 0,
        compactionLevel: blockExec.compaction_level ?? 0,
      };
      stats.blockExecutions!.push(blockInfo);
    }

    tree.stats = stats;
  }

  tree.children = (node.children || [])
    .map((child) => convertExecutionNodeToTreeWithBase(child, queryStartNs))
    .filter((n): n is ExecutionTreeNode => n !== null);

  return tree;
}
