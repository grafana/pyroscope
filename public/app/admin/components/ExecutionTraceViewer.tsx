import React from 'react';
import type { ExecutionTreeNode as ExecutionTreeNodeType } from '../types';
import { ExecutionTreeNode } from './ExecutionTreeNode';
import { formatMs } from '../utils';

interface ExecutionTraceViewerProps {
  executionTree: ExecutionTreeNodeType | null;
  responseTimeMs: number | null;
  diagnosticsId: string | null;
}

export function ExecutionTraceViewer({
  executionTree,
  responseTimeMs,
  diagnosticsId,
}: ExecutionTraceViewerProps) {
  if (!executionTree) {
    return null;
  }

  return (
    <div className="row mt-4">
      <div className="col-12">
        <div className="card">
          <div className="card-header d-flex justify-content-between align-items-center">
            <h5 className="mb-0">Execution Trace</h5>
            <div>
              {responseTimeMs != null && responseTimeMs > 0 && (
                <span className="badge bg-success">
                  Response time: {formatMs(responseTimeMs)}
                </span>
              )}
              {diagnosticsId && (
                <span className="badge bg-secondary font-monospace ms-2">
                  ID: {diagnosticsId}
                </span>
              )}
            </div>
          </div>
          <div className="card-body">
            <div className="execution-tree">
              <ExecutionTreeNode node={executionTree} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
