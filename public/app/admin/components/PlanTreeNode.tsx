import React from 'react';
import type { PlanTreeNode as PlanTreeNodeType } from '../types';

interface PlanTreeNodeProps {
  node: PlanTreeNodeType;
}

export function PlanTreeNode({ node }: PlanTreeNodeProps) {
  return (
    <div className={`tree-node tree-node-${node.type.toLowerCase()}`}>
      <div className="tree-node-header">
        {node.type === 'MERGE' ? (
          <>
            <i className="bi bi-arrows-collapse tree-node-icon"></i>
            <span className="tree-node-label">MERGE</span>
            <span className="badge bg-primary tree-node-badge">
              {node.children?.length || 0} children
            </span>
            <span className="badge bg-secondary tree-node-badge">
              {node.totalBlocks} blocks total
            </span>
          </>
        ) : node.type === 'READ' ? (
          <>
            <i className="bi bi-database tree-node-icon"></i>
            <span className="tree-node-label">READ</span>
            <span className="badge bg-success tree-node-badge">
              {node.blockCount} blocks
            </span>
          </>
        ) : (
          <>
            <i className="bi bi-question-circle tree-node-icon"></i>
            <span className="tree-node-label">{node.type}</span>
          </>
        )}
      </div>
      {node.type === 'READ' && node.blocks && node.blocks.length > 0 && (
        <div className="tree-node-blocks">
          {node.blocks.slice(0, 5).map((block, idx) => (
            <div key={idx}>
              <code>{block.id}</code>{' '}
              <span className="text-muted">
                (shard {block.shard}, L{block.compactionLevel}, {block.size})
              </span>
            </div>
          ))}
          {node.blockCount > 5 && (
            <div>
              <em>... and {node.blockCount - 5} more</em>
            </div>
          )}
        </div>
      )}
      {node.children && node.children.length > 0 && (
        <div className="tree-node-children">
          {node.children.map((child, idx) => (
            <PlanTreeNode key={idx} node={child} />
          ))}
        </div>
      )}
    </div>
  );
}
