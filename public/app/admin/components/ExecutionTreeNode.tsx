import React from 'react';
import type { ExecutionTreeNode as ExecutionTreeNodeType } from '../types';

interface ExecutionTreeNodeProps {
  node: ExecutionTreeNodeType;
}

export function ExecutionTreeNode({ node }: ExecutionTreeNodeProps) {
  return (
    <div className={`exec-node exec-node-${node.type.toLowerCase()}`}>
      <div className="exec-node-header">
        <span className="exec-type">{node.type}</span>
        <span className="badge bg-secondary">{node.executor}</span>
        <span className="badge bg-warning text-dark" title="Started at">
          @{node.relativeStartStr}
        </span>
        <span className="badge bg-info" title="Duration">
          {node.durationStr}
        </span>
        {node.stats && (
          <span className="badge bg-light text-dark">
            {node.stats.blocksRead} blocks, {node.stats.datasetsProcessed}{' '}
            datasets
          </span>
        )}
      </div>
      {node.error && (
        <div className="exec-error text-danger">
          <small>Error: {node.error}</small>
        </div>
      )}
      {node.stats?.blockExecutions && node.stats.blockExecutions.length > 0 && (
        <div className="exec-blocks mt-2">
          <table
            className="table table-sm table-hover mb-0"
            style={{ fontSize: '0.8rem' }}
          >
            <thead>
              <tr>
                <th>Block ID</th>
                <th>Start</th>
                <th>Duration</th>
                <th>End</th>
                <th>Datasets</th>
                <th>Size</th>
                <th>Shard</th>
                <th>Level</th>
              </tr>
            </thead>
            <tbody>
              {node.stats.blockExecutions.map((blockExec, idx) => (
                <tr key={idx}>
                  <td>
                    <code>{blockExec.blockId}</code>
                  </td>
                  <td>
                    <span className="badge bg-warning text-dark">
                      @{blockExec.relativeStartStr}
                    </span>
                  </td>
                  <td>
                    <span className="badge bg-info">
                      {blockExec.durationStr}
                    </span>
                  </td>
                  <td>
                    <span className="badge bg-secondary">
                      @{blockExec.relativeEndStr}
                    </span>
                  </td>
                  <td>{blockExec.datasetsProcessed}</td>
                  <td>{blockExec.size}</td>
                  <td>{blockExec.shard}</td>
                  <td>L{blockExec.compactionLevel}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {node.children && node.children.length > 0 && (
        <div className="exec-children">
          {node.children.map((child, idx) => (
            <ExecutionTreeNode key={idx} node={child} />
          ))}
        </div>
      )}
    </div>
  );
}
