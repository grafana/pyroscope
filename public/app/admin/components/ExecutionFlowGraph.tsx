import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  Handle,
  Position,
} from '@xyflow/react';
import type { Node, Edge, NodeProps } from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import type { ExecutionTreeNode, BlockExecutionInfo } from '../types';

interface ExecutionFlowGraphProps {
  executionTree: ExecutionTreeNode;
}

interface ExecutionNodeData extends Record<string, unknown> {
  label: string;
  nodeType: string;
  executor: string;
  durationStr: string;
  relativeStartStr: string;
  duration: number;
  relativeStart: number;
  stats?: {
    blocksRead: number;
    datasetsProcessed: number;
    blockExecutions?: BlockExecutionInfo[];
  };
  error?: string;
}

type ExecutionFlowNode = Node<ExecutionNodeData, 'executionNode'>;

const NODE_WIDTH = 240;
const NODE_HEIGHT = 65;
const HORIZONTAL_GAP = 40;
const VERTICAL_GAP = 50;
const GRID_THRESHOLD = 4;

function ExecutionNode({ data, selected }: NodeProps<ExecutionFlowNode>) {
  const nodeClass = data.nodeType === 'MERGE' ? 'merge' : 'read';

  return (
    <div className={`execution-flow-node ${nodeClass} ${selected ? 'selected' : ''}`}>
      <Handle type="target" position={Position.Top} />
      <div className="node-header">
        <span className={`node-type ${nodeClass}`}>{data.nodeType}</span>
        <span className="node-executor">{data.executor}</span>
      </div>
      <div className="node-timing">
        <span className="node-start" title="Started at">@{data.relativeStartStr}</span>
        <span className="node-duration" title="Duration">{data.durationStr}</span>
      </div>
      {data.error && <div className="node-error">Error</div>}
      <Handle type="source" position={Position.Bottom} />
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

  if (childCount > GRID_THRESHOLD) {
    const cols = Math.ceil(Math.sqrt(childCount));
    const rows = Math.ceil(childCount / cols);

    const maxChildWidth = Math.max(...node.children.map(c => c.subtreeWidth));
    const maxChildHeight = Math.max(...node.children.map(c => c.subtreeHeight));

    const gridWidth = cols * maxChildWidth + (cols - 1) * HORIZONTAL_GAP;
    const gridHeight = rows * maxChildHeight + (rows - 1) * VERTICAL_GAP;

    node.subtreeWidth = Math.max(node.width, gridWidth);
    node.subtreeHeight = node.height + VERTICAL_GAP + gridHeight;
  } else {
    const totalChildWidth = node.children.reduce((sum, c) => sum + c.subtreeWidth, 0)
      + (childCount - 1) * HORIZONTAL_GAP;
    const maxChildHeight = Math.max(...node.children.map(c => c.subtreeHeight));

    node.subtreeWidth = Math.max(node.width, totalChildWidth);
    node.subtreeHeight = node.height + VERTICAL_GAP + maxChildHeight;
  }
}

function positionNodes(node: LayoutNode, startX: number, startY: number): void {
  node.x = startX + (node.subtreeWidth - node.width) / 2;
  node.y = startY;

  if (node.children.length === 0) {
    return;
  }

  const childStartY = startY + node.height + VERTICAL_GAP;
  const childCount = node.children.length;

  if (childCount > GRID_THRESHOLD) {
    const cols = Math.ceil(Math.sqrt(childCount));

    const maxChildWidth = Math.max(...node.children.map(c => c.subtreeWidth));
    const maxChildHeight = Math.max(...node.children.map(c => c.subtreeHeight));

    const gridWidth = cols * maxChildWidth + (cols - 1) * HORIZONTAL_GAP;
    const gridStartX = startX + (node.subtreeWidth - gridWidth) / 2;

    for (let i = 0; i < node.children.length; i++) {
      const child = node.children[i];
      const col = i % cols;
      const row = Math.floor(i / cols);

      const cellX = gridStartX + col * (maxChildWidth + HORIZONTAL_GAP);
      const cellY = childStartY + row * (maxChildHeight + VERTICAL_GAP);

      const childOffsetX = (maxChildWidth - child.subtreeWidth) / 2;
      positionNodes(child, cellX + childOffsetX, cellY);
    }
  } else {
    const totalChildWidth = node.children.reduce((sum, c) => sum + c.subtreeWidth, 0)
      + (childCount - 1) * HORIZONTAL_GAP;
    let currentX = startX + (node.subtreeWidth - totalChildWidth) / 2;

    for (const child of node.children) {
      positionNodes(child, currentX, childStartY);
      currentX += child.subtreeWidth + HORIZONTAL_GAP;
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
      type: 'smoothstep',
      style: { stroke: '#6c757d', strokeWidth: 2 },
    });
  }

  for (const child of node.children) {
    flattenLayoutTree(child, nodes, edges, node.id);
  }
}

function layoutTree(tree: ExecutionTreeNode): { nodes: ExecutionFlowNode[]; edges: Edge[] } {
  const layoutRoot = buildLayoutTree(tree);
  calculateSubtreeSizes(layoutRoot);
  positionNodes(layoutRoot, 0, 0);

  const nodes: ExecutionFlowNode[] = [];
  const edges: Edge[] = [];
  flattenLayoutTree(layoutRoot, nodes, edges);

  return { nodes, edges };
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

export function ExecutionFlowGraph({ executionTree }: ExecutionFlowGraphProps) {
  const { nodes: initialNodes, edges: initialEdges } = useMemo(() => {
    return layoutTree(executionTree);
  }, [executionTree]);

  const [nodes, setNodes, onNodesChange] = useNodesState<ExecutionFlowNode>(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const [selectedNode, setSelectedNode] = useState<ExecutionFlowNode | null>(null);

  useEffect(() => {
    const { nodes: newNodes, edges: newEdges } = layoutTree(executionTree);
    setNodes(newNodes);
    setEdges(newEdges);
    setSelectedNode(null);
  }, [executionTree, setNodes, setEdges]);

  const onNodeClick = useCallback((_event: React.MouseEvent, node: ExecutionFlowNode) => {
    setSelectedNode(node);
  }, []);

  const handleClosePanel = useCallback(() => {
    setSelectedNode(null);
  }, []);

  const minimapNodeColor = useCallback((node: ExecutionFlowNode) => {
    return node.data.nodeType === 'MERGE' ? '#0d6efd' : '#198754';
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
          <MiniMap
            nodeColor={minimapNodeColor}
            nodeStrokeWidth={3}
            zoomable
            pannable
            maskColor="rgba(0, 0, 0, 0.7)"
          />
        </ReactFlow>
      </div>
      <NodeDetailsPanel node={selectedNode} onClose={handleClosePanel} />
    </div>
  );
}
