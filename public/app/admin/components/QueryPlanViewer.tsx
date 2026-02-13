import React, { useCallback, useMemo, useState } from 'react';
import type {
  PlanTreeNode as PlanTreeNodeType,
  RawQueryPlan,
} from '../types';
import { PlanTreeNode } from './PlanTreeNode';
import { exportBlocks } from '../services/api';

function countL3Blocks(plan: RawQueryPlan | null): number {
  if (!plan || !plan.root) {
    return 0;
  }
  const seen = new Set<string>();
  countL3BlocksFromNode(plan.root, seen);
  return seen.size;
}

function countL3BlocksFromNode(
  node: { type: number | string; children?: unknown[]; blocks?: { id: string; compaction_level: number }[] },
  seen: Set<string>
): void {
  if (!node) {
    return;
  }
  // READ node type = 2
  if ((node.type === 2 || node.type === 'READ') && node.blocks) {
    for (const b of node.blocks) {
      if (b.compaction_level === 3) {
        seen.add(b.id);
      }
    }
  }
  for (const child of (node.children || []) as typeof node[]) {
    countL3BlocksFromNode(child, seen);
  }
}

interface QueryPlanViewerProps {
  planTree: PlanTreeNodeType | null;
  planJson: string | null;
  planRaw: RawQueryPlan | null;
  metadataStats: string | null;
  tenantId?: string;
  diagnosticsId?: string | null;
}

export function QueryPlanViewer({
  planTree,
  planJson,
  planRaw,
  metadataStats,
  tenantId,
  diagnosticsId,
}: QueryPlanViewerProps) {
  const [activeTab, setActiveTab] = useState<'visual' | 'json'>('visual');
  const [isExporting, setIsExporting] = useState(false);

  const l3BlockCount = useMemo(() => countL3Blocks(planRaw), [planRaw]);

  const handleExportBlocks = useCallback(async () => {
    if (!diagnosticsId || !tenantId) {
      return;
    }

    setIsExporting(true);
    try {
      const blob = await exportBlocks(tenantId, diagnosticsId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `blocks-${tenantId}-${diagnosticsId}.tar.gz`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error('Failed to export blocks:', err);
      alert(
        'Failed to export blocks: ' +
          (err instanceof Error ? err.message : String(err))
      );
    } finally {
      setIsExporting(false);
    }
  }, [diagnosticsId, tenantId]);

  if (!planTree && !metadataStats) {
    return (
      <div className="card">
        <div className="card-header">
          <h5 className="mb-0">Results</h5>
        </div>
        <div className="card-body text-muted">
          <p>Execute a query to see the query plan and execution trace.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="card">
      <div className="card-header d-flex justify-content-between align-items-center">
        <h5 className="mb-0">Query Plan</h5>
        {diagnosticsId && tenantId && l3BlockCount > 0 && (
          <button
            type="button"
            className="btn btn-sm btn-outline-primary"
            onClick={handleExportBlocks}
            disabled={isExporting}
            title="Export anonymized L3 blocks as tar.gz"
          >
            {isExporting
              ? 'Exporting...'
              : `Export L3 Blocks (${l3BlockCount})`}
          </button>
        )}
      </div>
      <div className="card-body">
        {metadataStats && (
          <div className="stats-box mb-3">
            <strong>Metadata Stats:</strong>
            <pre className="mb-0 mt-2">{metadataStats}</pre>
          </div>
        )}

        {planTree && (
          <>
            <ul className="nav nav-tabs mb-3" role="tablist">
              <li className="nav-item" role="presentation">
                <button
                  className={`nav-link ${
                    activeTab === 'visual' ? 'active' : ''
                  }`}
                  onClick={() => setActiveTab('visual')}
                  type="button"
                  role="tab"
                >
                  <i className="bi bi-diagram-3"></i> Visual
                </button>
              </li>
              <li className="nav-item" role="presentation">
                <button
                  className={`nav-link ${activeTab === 'json' ? 'active' : ''}`}
                  onClick={() => setActiveTab('json')}
                  type="button"
                  role="tab"
                >
                  <i className="bi bi-code-slash"></i> JSON
                </button>
              </li>
            </ul>
            <div className="tab-content">
              {activeTab === 'visual' && (
                <div className="plan-tree">
                  <PlanTreeNode node={planTree} />
                </div>
              )}
              {activeTab === 'json' && (
                <textarea
                  className="form-control plan-textarea"
                  rows={15}
                  readOnly
                  value={planJson || ''}
                />
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
