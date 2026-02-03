import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  Handle,
  Position,
  Panel,
} from '@xyflow/react';
import type { Node, Edge, NodeProps } from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import type { ExecutionTreeNode, BlockExecutionInfo } from '../types';

interface ExecutionFlowGraphProps {
  executionTree: ExecutionTreeNode;
  responseTimeMs: number | null;
}

type AnimationState = 'pending' | 'active' | 'completed';

interface ExecutionNodeData extends Record<string, unknown> {
  label: string;
  nodeType: string;
  executor: string;
  durationStr: string;
  relativeStartStr: string;
  duration: number;
  relativeStart: number;
  animationState: AnimationState;
  stats?: {
    blocksRead: number;
    datasetsProcessed: number;
    blockExecutions?: BlockExecutionInfo[];
  };
  error?: string;
}

type ExecutionFlowNode = Node<ExecutionNodeData, 'executionNode'>;

const NODE_WIDTH = 140;
const NODE_HEIGHT = 44;
const HORIZONTAL_GAP = 60;
const VERTICAL_GAP = 15;
const LEAF_GRID_THRESHOLD = 6;
const LEAF_GRID_COLUMNS = 4;

function getNodeClass(nodeType: string): string {
  switch (nodeType) {
    case 'MERGE':
      return 'merge';
    case 'READ':
      return 'read';
    case 'FRONTEND':
      return 'frontend';
    default:
      return 'read';
  }
}

function shortenExecutorName(executor: string): string {
  // Handle k8s-style pod names: prefix-deployment-hash-podid
  // e.g., "fire-query-backend-0457f4f8d-lx55c" -> "lx55c"
  const parts = executor.split('-');
  if (parts.length >= 2) {
    // Return last part (pod ID) if it looks like a hash
    const lastPart = parts[parts.length - 1];
    if (/^[a-z0-9]{4,}$/i.test(lastPart)) {
      return lastPart;
    }
  }
  // Fallback: return last 12 chars if too long
  if (executor.length > 12) {
    return '…' + executor.slice(-11);
  }
  return executor;
}

function ExecutionNode({ data, selected }: NodeProps<ExecutionFlowNode>) {
  const nodeClass = getNodeClass(data.nodeType);
  const animClass = data.animationState || 'pending';
  const isFrontend = data.nodeType === 'FRONTEND';
  const shortName = isFrontend ? data.executor : shortenExecutorName(data.executor);

  return (
    <div className={`execution-flow-node ${nodeClass} ${animClass} ${selected ? 'selected' : ''}`}>
      {!isFrontend && <Handle type="target" position={Position.Left} />}
      <div className="node-header">
        <span className="node-executor" title={data.executor}>{shortName}</span>
      </div>
      <div className="node-timing">
        <span className="node-start" title="Started at">@{data.relativeStartStr}</span>
        <span className="node-duration" title="Duration">{data.durationStr}</span>
      </div>
      {data.error && <div className="node-error">Error</div>}
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

const nodeTypes = {
  executionNode: ExecutionNode,
};

interface LayoutNode {
  id: string;
  data: ExecutionNodeData;
  children: LayoutNode[];
  width: number;
  height: number;
  x: number;
  y: number;
  subtreeWidth: number;
  subtreeHeight: number;
}

function buildLayoutTree(tree: ExecutionTreeNode, idCounter = { value: 0 }): LayoutNode {
  const nodeId = `node-${idCounter.value++}`;

  const children: LayoutNode[] = (tree.children || []).map(child =>
    buildLayoutTree(child, idCounter)
  );

  return {
    id: nodeId,
    data: {
      label: `${tree.type} - ${tree.executor}`,
      nodeType: tree.type,
      executor: tree.executor,
      durationStr: tree.durationStr,
      relativeStartStr: tree.relativeStartStr,
      duration: tree.duration,
      relativeStart: tree.relativeStart,
      animationState: 'pending' as AnimationState,
      stats: tree.stats,
      error: tree.error,
    },
    children,
    width: NODE_WIDTH,
    height: NODE_HEIGHT,
    x: 0,
    y: 0,
    subtreeWidth: 0,
    subtreeHeight: 0,
  };
}

function isLeafParent(node: LayoutNode): boolean {
  return node.children.length > 0 && node.children.every(c => c.children.length === 0);
}

function shouldUseLeafGrid(node: LayoutNode): boolean {
  return isLeafParent(node) && node.children.length >= LEAF_GRID_THRESHOLD;
}

function calculateSubtreeSizes(node: LayoutNode): void {
  if (node.children.length === 0) {
    node.subtreeWidth = node.width;
    node.subtreeHeight = node.height;
    return;
  }

  for (const child of node.children) {
    calculateSubtreeSizes(child);
  }

  const childCount = node.children.length;

  if (shouldUseLeafGrid(node)) {
    const cols = LEAF_GRID_COLUMNS;
    const rows = Math.ceil(childCount / cols);

    const gridWidth = cols * NODE_WIDTH + (cols - 1) * HORIZONTAL_GAP;
    const gridHeight = rows * NODE_HEIGHT + (rows - 1) * VERTICAL_GAP;

    node.subtreeWidth = node.width + HORIZONTAL_GAP + gridWidth;
    node.subtreeHeight = Math.max(node.height, gridHeight);
  } else {
    const totalChildHeight = node.children.reduce((sum, c) => sum + c.subtreeHeight, 0)
      + (childCount - 1) * VERTICAL_GAP;
    const maxChildWidth = Math.max(...node.children.map(c => c.subtreeWidth));

    node.subtreeWidth = node.width + HORIZONTAL_GAP + maxChildWidth;
    node.subtreeHeight = Math.max(node.height, totalChildHeight);
  }
}

function positionNodes(node: LayoutNode, startX: number, startY: number): void {
  node.x = startX;
  node.y = startY + (node.subtreeHeight - node.height) / 2;

  if (node.children.length === 0) {
    return;
  }

  const childStartX = startX + node.width + HORIZONTAL_GAP;
  const childCount = node.children.length;

  if (shouldUseLeafGrid(node)) {
    const cols = LEAF_GRID_COLUMNS;
    const rows = Math.ceil(childCount / cols);

    const gridHeight = rows * NODE_HEIGHT + (rows - 1) * VERTICAL_GAP;
    const gridStartY = startY + (node.subtreeHeight - gridHeight) / 2;

    for (let i = 0; i < node.children.length; i++) {
      const child = node.children[i];
      const col = Math.floor(i / rows);
      const row = i % rows;

      child.x = childStartX + col * (NODE_WIDTH + HORIZONTAL_GAP);
      child.y = gridStartY + row * (NODE_HEIGHT + VERTICAL_GAP);
    }
  } else {
    const totalChildHeight = node.children.reduce((sum, c) => sum + c.subtreeHeight, 0)
      + (childCount - 1) * VERTICAL_GAP;
    let currentY = startY + (node.subtreeHeight - totalChildHeight) / 2;

    for (const child of node.children) {
      positionNodes(child, childStartX, currentY);
      currentY += child.subtreeHeight + VERTICAL_GAP;
    }
  }
}

function flattenLayoutTree(
  node: LayoutNode,
  nodes: ExecutionFlowNode[],
  edges: Edge[],
  parentId: string | null = null
): void {
  const flowNode: ExecutionFlowNode = {
    id: node.id,
    type: 'executionNode',
    position: { x: node.x, y: node.y },
    data: node.data,
  };

  nodes.push(flowNode);

  if (parentId) {
    edges.push({
      id: `edge-${parentId}-${node.id}`,
      source: parentId,
      target: node.id,
      type: 'default',
      pathOptions: { curvature: 0.4 },
      style: { stroke: '#6c757d', strokeWidth: 1.5 },
    });
  }

  for (const child of node.children) {
    flattenLayoutTree(child, nodes, edges, node.id);
  }
}

function formatDurationNs(ns: number): string {
  const ms = ns / NS_PER_MS;
  if (ms < 1) {
    return `${(ms * 1000).toFixed(0)}µs`;
  }
  if (ms < 1000) {
    return `${ms.toFixed(2)}ms`;
  }
  return `${(ms / 1000).toFixed(2)}s`;
}

function layoutTree(
  tree: ExecutionTreeNode,
  responseTimeMs: number | null
): { nodes: ExecutionFlowNode[]; edges: Edge[] } {
  const layoutRoot = buildLayoutTree(tree);

  // Wrap with frontend node if we have response time
  let rootToLayout: LayoutNode;
  if (responseTimeMs != null && responseTimeMs > 0) {
    const frontendDurationNs = responseTimeMs * NS_PER_MS;
    const frontendNode: LayoutNode = {
      id: 'frontend',
      data: {
        label: 'Query Frontend',
        nodeType: 'FRONTEND',
        executor: 'query-frontend',
        durationStr: formatDurationNs(frontendDurationNs),
        relativeStartStr: '0.00ms',
        duration: frontendDurationNs,
        relativeStart: 0,
        animationState: 'pending' as AnimationState,
      },
      children: [layoutRoot],
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
      x: 0,
      y: 0,
      subtreeWidth: 0,
      subtreeHeight: 0,
    };
    rootToLayout = frontendNode;
  } else {
    rootToLayout = layoutRoot;
  }

  calculateSubtreeSizes(rootToLayout);
  positionNodes(rootToLayout, 0, 0);

  const nodes: ExecutionFlowNode[] = [];
  const edges: Edge[] = [];
  flattenLayoutTree(rootToLayout, nodes, edges);

  return { nodes, edges };
}

function calculateTotalDuration(nodes: ExecutionFlowNode[]): number {
  let maxEnd = 0;
  for (const node of nodes) {
    const end = node.data.relativeStart + node.data.duration;
    if (end > maxEnd) {
      maxEnd = end;
    }
  }
  return maxEnd;
}

function getNodeAnimationState(node: ExecutionFlowNode, currentTime: number): AnimationState {
  const start = node.data.relativeStart;
  const end = start + node.data.duration;

  if (currentTime < start) {
    return 'pending';
  } else if (currentTime < end) {
    return 'active';
  } else {
    return 'completed';
  }
}

function getEdgeStyleFromState(state: AnimationState): { stroke: string; strokeWidth: number } {
  switch (state) {
    case 'active':
      return { stroke: '#ffc107', strokeWidth: 2.5 };
    case 'completed':
      return { stroke: '#198754', strokeWidth: 2 };
    default:
      return { stroke: '#6c757d', strokeWidth: 1.5 };
  }
}

function NodeDetailsPanel({
  node,
  onClose,
}: {
  node: ExecutionFlowNode | null;
  onClose: () => void;
}) {
  if (!node) {
    return null;
  }

  const { data } = node;

  return (
    <div className="node-details-panel">
      <div className="panel-header">
        <h6>Node Details</h6>
        <button type="button" className="btn-close" onClick={onClose} aria-label="Close" />
      </div>
      <div className="panel-body">
        <div className="detail-row">
          <label>Type:</label>
          <span className={`badge bg-${data.nodeType === 'MERGE' ? 'primary' : 'success'}`}>
            {data.nodeType}
          </span>
        </div>
        <div className="detail-row">
          <label>Executor:</label>
          <code>{data.executor}</code>
        </div>
        <div className="detail-row">
          <label>Started at:</label>
          <span className="badge bg-warning text-dark">@{data.relativeStartStr}</span>
        </div>
        <div className="detail-row">
          <label>Duration:</label>
          <span className="badge bg-info">{data.durationStr}</span>
        </div>

        {data.error && (
          <div className="detail-row error-row">
            <label>Error:</label>
            <span className="text-danger">{data.error}</span>
          </div>
        )}

        {data.stats && (
          <>
            <div className="detail-row">
              <label>Blocks Read:</label>
              <span>{data.stats.blocksRead}</span>
            </div>
            <div className="detail-row">
              <label>Datasets Processed:</label>
              <span>{data.stats.datasetsProcessed}</span>
            </div>

            {data.stats.blockExecutions && data.stats.blockExecutions.length > 0 && (
              <div className="block-executions">
                <h6>Block Executions</h6>
                <div className="block-table-wrapper">
                  <table className="table table-sm">
                    <thead>
                      <tr>
                        <th>Block ID</th>
                        <th>Start</th>
                        <th>Duration</th>
                        <th>Datasets</th>
                        <th>Size</th>
                        <th>Shard</th>
                        <th>Level</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.stats.blockExecutions.map((block, idx) => (
                        <tr key={idx}>
                          <td><code>{block.blockId}</code></td>
                          <td>
                            <span className="badge bg-warning text-dark">
                              @{block.relativeStartStr}
                            </span>
                          </td>
                          <td>
                            <span className="badge bg-info">{block.durationStr}</span>
                          </td>
                          <td>{block.datasetsProcessed}</td>
                          <td>{block.size}</td>
                          <td>{block.shard}</td>
                          <td>L{block.compactionLevel}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

const NS_PER_MS = 1_000_000; // Nanoseconds per millisecond
const TICK_INTERVAL = 50; // Check every 50ms
const SPEED_OPTIONS = [0.1, 0.25, 0.5, 1, 2, 5] as const;

function formatTime(ns: number): string {
  const ms = ns / NS_PER_MS;
  if (ms < 1000) {
    return `${ms.toFixed(0)}ms`;
  }
  return `${(ms / 1000).toFixed(2)}s`;
}

interface PlaybackControlsProps {
  isPlaying: boolean;
  speed: number;
  currentTime: number;
  totalDuration: number;
  onPlayPause: () => void;
  onSpeedChange: (speed: number) => void;
  onSeek: (time: number) => void;
  onRestart: () => void;
}

function PlaybackControls({
  isPlaying,
  speed,
  currentTime,
  totalDuration,
  onPlayPause,
  onSpeedChange,
  onSeek,
  onRestart,
}: PlaybackControlsProps) {
  const progress = totalDuration > 0 ? (currentTime / totalDuration) * 100 : 0;

  return (
    <div className="playback-controls">
      <button
        className="playback-btn"
        onClick={onRestart}
        title="Restart"
      >
        ⏮
      </button>
      <button
        className="playback-btn play-pause"
        onClick={onPlayPause}
        title={isPlaying ? 'Pause' : 'Play'}
      >
        {isPlaying ? '⏸' : '▶'}
      </button>
      <div className="playback-progress">
        <input
          type="range"
          min="0"
          max="100"
          value={progress}
          onChange={(e) => onSeek((parseFloat(e.target.value) / 100) * totalDuration)}
          className="progress-slider"
        />
        <span className="time-display">
          {formatTime(currentTime)} / {formatTime(totalDuration)}
        </span>
      </div>
      <div className="speed-control">
        <select
          value={speed}
          onChange={(e) => onSpeedChange(parseFloat(e.target.value))}
          className="speed-select"
        >
          {SPEED_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {s}x
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}

export function ExecutionFlowGraph({ executionTree, responseTimeMs }: ExecutionFlowGraphProps) {
  const { nodes: initialNodes, edges: initialEdges } = useMemo(() => {
    return layoutTree(executionTree, responseTimeMs);
  }, [executionTree, responseTimeMs]);

  const [nodes, setNodes, onNodesChange] = useNodesState<ExecutionFlowNode>(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const [selectedNode, setSelectedNode] = useState<ExecutionFlowNode | null>(null);

  const [isPlaying, setIsPlaying] = useState(true); // Auto-play on load
  const [speed, setSpeed] = useState(1);
  const [currentTime, setCurrentTime] = useState(0);

  const baseNodesRef = useRef<ExecutionFlowNode[]>(initialNodes);
  const baseEdgesRef = useRef<Edge[]>(initialEdges);
  const totalDurationRef = useRef<number>(calculateTotalDuration(initialNodes));
  const currentTimeRef = useRef<number>(0);
  const nodeStatesRef = useRef<Map<string, AnimationState>>(new Map());

  useEffect(() => {
    const { nodes: newNodes, edges: newEdges } = layoutTree(executionTree, responseTimeMs);
    baseNodesRef.current = newNodes;
    baseEdgesRef.current = newEdges;
    totalDurationRef.current = calculateTotalDuration(newNodes);
    currentTimeRef.current = 0;
    nodeStatesRef.current = new Map();
    setCurrentTime(0);
    setIsPlaying(true); // Auto-play on new data
    setNodes(newNodes);
    setEdges(newEdges);
    setSelectedNode(null);
  }, [executionTree, responseTimeMs, setNodes, setEdges]);

  const updateAnimationState = useCallback((time: number) => {
    const baseNodes = baseNodesRef.current;
    const baseEdges = baseEdgesRef.current;
    const prevStates = nodeStatesRef.current;

    const newStates = new Map<string, AnimationState>();
    let hasChanges = false;

    for (const node of baseNodes) {
      const newState = getNodeAnimationState(node, time);
      newStates.set(node.id, newState);
      if (prevStates.get(node.id) !== newState) {
        hasChanges = true;
      }
    }

    if (hasChanges) {
      nodeStatesRef.current = newStates;

      setNodes(currentNodes =>
        currentNodes.map(node => ({
          ...node,
          data: {
            ...node.data,
            animationState: newStates.get(node.id) || 'pending',
          },
        }))
      );

      setEdges(currentEdges =>
        currentEdges.map(edge => {
          const targetState = newStates.get(edge.target) || 'pending';
          return {
            ...edge,
            animated: targetState === 'active',
            style: getEdgeStyleFromState(targetState),
          };
        })
      );
    }
  }, [setNodes, setEdges]);

  useEffect(() => {
    if (!isPlaying) {
      return;
    }

    const animate = () => {
      const totalDuration = totalDurationRef.current;
      const newTime = currentTimeRef.current + TICK_INTERVAL * speed * NS_PER_MS;

      if (newTime > totalDuration) {
        currentTimeRef.current = totalDuration;
        setCurrentTime(totalDuration);
        updateAnimationState(totalDuration);
        setIsPlaying(false); // Stop at end, no looping
        return;
      }

      currentTimeRef.current = newTime;
      setCurrentTime(newTime);
      updateAnimationState(newTime);
    };

    const intervalId = setInterval(animate, TICK_INTERVAL);
    return () => clearInterval(intervalId);
  }, [isPlaying, speed, updateAnimationState]);

  const handlePlayPause = useCallback(() => {
    setIsPlaying(prev => !prev);
  }, []);

  const handleSpeedChange = useCallback((newSpeed: number) => {
    setSpeed(newSpeed);
  }, []);

  const handleSeek = useCallback((time: number) => {
    currentTimeRef.current = time;
    setCurrentTime(time);
    updateAnimationState(time);
  }, [updateAnimationState]);

  const handleRestart = useCallback(() => {
    currentTimeRef.current = 0;
    nodeStatesRef.current = new Map();
    setCurrentTime(0);
    updateAnimationState(0);
    setIsPlaying(true);
  }, [updateAnimationState]);

  const onNodeClick = useCallback((_event: React.MouseEvent, node: ExecutionFlowNode) => {
    setSelectedNode(node);
  }, []);

  const handleClosePanel = useCallback(() => {
    setSelectedNode(null);
  }, []);

  return (
    <div className="execution-flow-container">
      <div className={`flow-wrapper ${selectedNode ? 'with-panel' : ''}`}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={onNodeClick}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          minZoom={0.1}
          maxZoom={2}
          colorMode="dark"
        >
          <Background color="#444" gap={16} />
          <Controls />
          <Panel position="bottom-center">
            <PlaybackControls
              isPlaying={isPlaying}
              speed={speed}
              currentTime={currentTime}
              totalDuration={totalDurationRef.current}
              onPlayPause={handlePlayPause}
              onSpeedChange={handleSpeedChange}
              onSeek={handleSeek}
              onRestart={handleRestart}
            />
          </Panel>
        </ReactFlow>
      </div>
      <NodeDetailsPanel node={selectedNode} onClose={handleClosePanel} />
    </div>
  );
}
