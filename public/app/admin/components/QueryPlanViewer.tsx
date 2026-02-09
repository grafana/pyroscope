import React, { useState } from 'react';
import type { PlanTreeNode as PlanTreeNodeType } from '../types';
import { PlanTreeNode } from './PlanTreeNode';

interface QueryPlanViewerProps {
  planTree: PlanTreeNodeType | null;
  planJson: string | null;
  metadataStats: string | null;
}

export function QueryPlanViewer({
  planTree,
  planJson,
  metadataStats,
}: QueryPlanViewerProps) {
  const [activeTab, setActiveTab] = useState<'visual' | 'json'>('visual');

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
