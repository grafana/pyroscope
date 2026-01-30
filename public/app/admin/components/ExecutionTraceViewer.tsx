import React, { useCallback, useState } from 'react';

import type { ExecutionTreeNode as ExecutionTreeNodeType } from '../types';
import { ExecutionFlowGraph } from './ExecutionFlowGraph';
import { formatMs } from '../utils';
import { exportDiagnostic } from '../services/api';

interface ExecutionTraceViewerProps {
  executionTree: ExecutionTreeNodeType | null;
  responseTimeMs: number | null;
  diagnosticsId: string | null;
  tenantId?: string;
}

export function ExecutionTraceViewer({
  executionTree,
  responseTimeMs,
  diagnosticsId,
  tenantId,
}: ExecutionTraceViewerProps) {
  const [isExporting, setIsExporting] = useState(false);

  const handleExport = useCallback(async () => {
    if (!diagnosticsId || !tenantId) {
      return;
    }

    setIsExporting(true);
    try {
      const blob = await exportDiagnostic(tenantId, diagnosticsId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `diagnostic-${tenantId}-${diagnosticsId}.zip`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error('Failed to export diagnostic:', err);
      alert('Failed to export diagnostic: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsExporting(false);
    }
  }, [diagnosticsId, tenantId]);

  if (!executionTree) {
    return (
      <div className="execution-trace-empty">
        <div className="text-muted text-center p-4">
          <p className="mb-0">No execution trace available.</p>
          <small>Run a query to see the execution trace.</small>
        </div>
      </div>
    );
  }

  return (
    <div className="execution-trace-container">
      <div className="execution-trace-header">
        <h5 className="mb-0">Execution Trace</h5>
        <div className="d-flex align-items-center gap-2">
          {responseTimeMs != null && responseTimeMs > 0 && (
            <span className="badge bg-success">
              Response time: {formatMs(responseTimeMs)}
            </span>
          )}
          {diagnosticsId && (
            <span className="badge bg-secondary font-monospace">
              ID: {diagnosticsId}
            </span>
          )}
          {diagnosticsId && tenantId && (
            <button
              type="button"
              className="btn btn-sm btn-outline-primary"
              onClick={handleExport}
              disabled={isExporting}
              title="Export diagnostic as zip file"
            >
              {isExporting ? 'Exporting...' : 'Export'}
            </button>
          )}
        </div>
      </div>
      <div className="execution-trace-body">
        <ExecutionFlowGraph executionTree={executionTree} />
      </div>
    </div>
  );
}
