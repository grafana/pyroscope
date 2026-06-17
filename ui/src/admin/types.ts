export type QueryMethod =
  | 'SelectMergeStacktraces'
  | 'SelectMergeProfile'
  | 'SelectMergeSpanProfile'
  | 'SelectSeries'
  | 'SelectHeatmap'
  | 'Diff'
  | 'LabelNames'
  | 'LabelValues'
  | 'Series'
  | 'ProfileTypes';

export const QUERY_METHODS: QueryMethod[] = [
  'SelectMergeStacktraces',
  'SelectMergeProfile',
  'SelectMergeSpanProfile',
  'SelectSeries',
  'SelectHeatmap',
  'Diff',
  'LabelNames',
  'LabelValues',
  'Series',
  'ProfileTypes',
];

export interface QueryParams {
  tenantId: string;
  method: QueryMethod;
  startTime: string;
  endTime: string;
  labelSelector: string;
  profileTypeId: string;
  maxNodes: string;
  format: string;
  spanSelector: string;
  labelName: string;
  labelNames: string;
  step: string;
  groupBy: string;
  aggregation: string;
  limit: string;
  heatmapQueryType: string;
  exemplarType: string;
  diffLeftSelector: string;
  diffLeftProfileType: string;
  diffLeftStart: string;
  diffLeftEnd: string;
  diffRightSelector: string;
  diffRightProfileType: string;
  diffRightStart: string;
  diffRightEnd: string;
}

export interface DiagnosticSummary {
  id: string;
  created_at: string;
  method: string;
  response_time_ms: number;
  response_size_bytes: number;
  request?: unknown;
}

// Raw diagnostic from API - uses snake_case JSON field names
export interface RawDiagnostic {
  id: string;
  tenant_id: string;
  created_at: string;
  method: string;
  response_time_ms: number;
  response_size_bytes: number;
  request?: unknown;
  plan?: RawQueryPlan;
  execution?: RawExecutionNode;
}

export interface RawQueryPlan {
  root?: RawQueryNode;
}

export interface RawQueryNode {
  type: number; // Protobuf enum: 0=MERGE, 1=READ
  children?: RawQueryNode[];
  blocks?: RawBlockMeta[];
}

export interface RawBlockMeta {
  id: string;
  shard: number;
  size: number;
  compaction_level: number;
  datasets?: RawDataset[];
  string_table?: string[];
}

export interface RawDataset {
  name: number;
  size: number;
}

export interface RawExecutionNode {
  type: number; // Protobuf enum
  executor: string;
  start_time_ns: number;
  end_time_ns: number;
  children?: RawExecutionNode[];
  stats?: RawExecutionStats;
  error?: string;
}

export interface RawExecutionStats {
  blocks_read: number;
  datasets_processed: number;
  block_executions?: RawBlockExecution[];
}

export interface RawBlockExecution {
  block_id: string;
  start_time_ns: number;
  end_time_ns: number;
  datasets_processed: number;
  size: number;
  shard: number;
  compaction_level: number;
}

// Processed types for display
export interface PlanTreeNode {
  type: 'MERGE' | 'READ';
  children?: PlanTreeNode[];
  blockCount: number;
  blocks: PlanTreeBlock[];
  totalBlocks: number;
}

export interface PlanTreeBlock {
  id: string;
  shard: number;
  size: string;
  compactionLevel: number;
}

export interface ExecutionTreeNode {
  type: string;
  executor: string;
  duration: number;
  durationStr: string;
  relativeStart: number;
  relativeStartStr: string;
  children?: ExecutionTreeNode[];
  stats?: ExecutionTreeStats;
  error?: string;
}

export interface ExecutionTreeStats {
  blocksRead: number;
  datasetsProcessed: number;
  blockExecutions?: BlockExecutionInfo[];
}

export interface BlockExecutionInfo {
  blockId: string;
  duration: number;
  durationStr: string;
  relativeStart: number;
  relativeStartStr: string;
  relativeEnd: number;
  relativeEndStr: string;
  datasetsProcessed: number;
  size: string;
  shard: number;
  compactionLevel: number;
}

// BlockMeta for metadata stats calculation
export interface BlockMeta {
  id: string;
  shard: number;
  size: number;
  compaction_level: number;
  datasets?: RawDataset[];
  string_table?: string[];
}
