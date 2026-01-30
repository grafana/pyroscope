import React, { useCallback, useEffect, useState } from 'react';
import { Header } from '../components/Header';
import { QueryForm } from '../components/QueryForm';
import { QueryPlanViewer } from '../components/QueryPlanViewer';
import { ExecutionTraceViewer } from '../components/ExecutionTraceViewer';
import { executeQuery, fetchTenants, loadDiagnostic } from '../services/api';
import {
  buildMetadataStats,
  convertExecutionNodeToTree,
  convertQueryPlanToTree,
  extractBlocksFromPlan,
} from '../utils';
import type {
  ExecutionTreeNode,
  PlanTreeNode,
  QueryParams,
  RawDiagnostic,
} from '../types';

function parseTimeForStats(timeStr: string): Date {
  if (!timeStr) return new Date();
  const now = Date.now();
  const match = timeStr.match(/^now(-(\d+)([smhd]))?$/);
  if (match) {
    if (!match[1]) return new Date(now);
    const value = parseInt(match[2], 10);
    const unit = match[3];
    const multipliers: Record<string, number> = {
      s: 1000,
      m: 60000,
      h: 3600000,
      d: 86400000,
    };
    return new Date(now - value * (multipliers[unit] || 0));
  }
  const d = new Date(timeStr);
  return isNaN(d.getTime()) ? new Date() : d;
}

function formatTimestamp(ms: number): string {
  return new Date(ms).toISOString();
}

interface RequestData {
  start?: number;
  end?: number;
  label_selector?: string;
  profile_typeID?: string;
  max_nodes?: number;
  format?: string;
  span_selector?: string[];
  step?: number;
  group_by?: string[];
  aggregation?: string;
  limit?: number;
  name?: string;
  label_names?: string[];
  matchers?: string[];
  query_type?: string;
  exemplar_type?: string;
  left?: {
    start?: number;
    end?: number;
    label_selector?: string;
    profile_typeID?: string;
  };
  right?: {
    start?: number;
    end?: number;
    label_selector?: string;
    profile_typeID?: string;
  };
}

function deserializeRequest(
  method: string,
  request: unknown
): Partial<QueryParams> {
  if (!request) return {};

  const req = request as RequestData;
  const params: Partial<QueryParams> = {};

  if (req.start) {
    params.startTime = formatTimestamp(req.start);
  }
  if (req.end) {
    params.endTime = formatTimestamp(req.end);
  }
  params.labelSelector = req.label_selector || '';
  params.profileTypeId = req.profile_typeID || '';

  switch (method) {
    case 'SelectMergeStacktraces':
    case 'SelectMergeProfile':
      if (req.max_nodes) {
        params.maxNodes = String(req.max_nodes);
      }
      if (req.format) {
        params.format = req.format;
      }
      break;

    case 'SelectMergeSpanProfile':
      if (req.max_nodes) {
        params.maxNodes = String(req.max_nodes);
      }
      if (req.format) {
        params.format = req.format;
      }
      if (req.span_selector && req.span_selector.length > 0) {
        params.spanSelector = req.span_selector.join(', ');
      }
      break;

    case 'SelectSeries':
      if (req.step) {
        params.step = String(req.step);
      }
      if (req.group_by && req.group_by.length > 0) {
        params.groupBy = req.group_by.join(', ');
      }
      if (req.aggregation) {
        params.aggregation = req.aggregation;
      }
      if (req.limit) {
        params.limit = String(req.limit);
      }
      if (req.exemplar_type) {
        params.exemplarType = req.exemplar_type;
      }
      break;

    case 'SelectHeatmap':
      if (req.step) {
        params.step = String(req.step);
      }
      if (req.group_by && req.group_by.length > 0) {
        params.groupBy = req.group_by.join(', ');
      }
      if (req.limit) {
        params.limit = String(req.limit);
      }
      if (req.query_type) {
        params.heatmapQueryType = req.query_type;
      }
      if (req.exemplar_type) {
        params.exemplarType = req.exemplar_type;
      }
      break;

    case 'Diff':
      if (req.left) {
        if (req.left.start) {
          params.diffLeftStart = formatTimestamp(req.left.start);
        }
        if (req.left.end) {
          params.diffLeftEnd = formatTimestamp(req.left.end);
        }
        params.diffLeftSelector = req.left.label_selector || '';
        params.diffLeftProfileType = req.left.profile_typeID || '';
      }
      if (req.right) {
        if (req.right.start) {
          params.diffRightStart = formatTimestamp(req.right.start);
        }
        if (req.right.end) {
          params.diffRightEnd = formatTimestamp(req.right.end);
        }
        params.diffRightSelector = req.right.label_selector || '';
        params.diffRightProfileType = req.right.profile_typeID || '';
      }
      break;

    case 'LabelNames':
      if (req.matchers && req.matchers.length > 0) {
        params.labelSelector = req.matchers.join(', ');
      }
      break;

    case 'LabelValues':
      if (req.name) {
        params.labelName = req.name;
      }
      if (req.matchers && req.matchers.length > 0) {
        params.labelSelector = req.matchers.join(', ');
      }
      break;

    case 'Series':
      if (req.matchers && req.matchers.length > 0) {
        params.labelSelector = req.matchers.join(', ');
      }
      if (req.label_names && req.label_names.length > 0) {
        params.labelNames = req.label_names.join(', ');
      }
      break;
  }

  return params;
}

const defaultParams: QueryParams = {
  tenantId: '',
  method: 'SelectMergeStacktraces',
  startTime: 'now-1h',
  endTime: 'now',
  labelSelector: '',
  profileTypeId: '',
  maxNodes: '',
  format: 'PROFILE_FORMAT_FLAMEGRAPH',
  spanSelector: '',
  labelName: '',
  labelNames: '',
  step: '',
  groupBy: '',
  aggregation: '',
  limit: '',
  heatmapQueryType: 'HEATMAP_QUERY_TYPE_INDIVIDUAL',
  exemplarType: '',
  diffLeftSelector: '',
  diffLeftProfileType: '',
  diffLeftStart: '',
  diffLeftEnd: '',
  diffRightSelector: '',
  diffRightProfileType: '',
  diffRightStart: '',
  diffRightEnd: '',
};

export function QueryDiagnosticsPage() {
  const [tenants, setTenants] = useState<string[]>([]);
  const [params, setParams] = useState<QueryParams>(defaultParams);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [globalError, setGlobalError] = useState<string | null>(null);

  const [planTree, setPlanTree] = useState<PlanTreeNode | null>(null);
  const [planJson, setPlanJson] = useState<string | null>(null);
  const [metadataStats, setMetadataStats] = useState<string | null>(null);
  const [executionTree, setExecutionTree] = useState<ExecutionTreeNode | null>(
    null
  );
  const [responseTimeMs, setResponseTimeMs] = useState<number | null>(null);
  const [diagnosticsId, setDiagnosticsId] = useState<string | null>(null);

  useEffect(() => {
    async function loadTenants() {
      try {
        const tenantList = await fetchTenants();
        setTenants(tenantList);
      } catch (err) {
        console.error('Failed to fetch tenants:', err);
      }
    }
    loadTenants();
  }, []);

  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const loadId = urlParams.get('load');
    const tenant = urlParams.get('tenant');

    if (loadId && tenant) {
      loadStoredDiagnostic(tenant, loadId);
    }
  }, []);

  const loadStoredDiagnostic = async (tenant: string, id: string) => {
    try {
      const diagnostic: RawDiagnostic = await loadDiagnostic(tenant, id);

      // Deserialize request to get form fields
      const requestParams = deserializeRequest(
        diagnostic.method,
        diagnostic.request
      );

      const newParams = {
        ...defaultParams,
        tenantId: diagnostic.tenant_id,
        method: (diagnostic.method as QueryParams['method']) || defaultParams.method,
        ...requestParams,
      };

      setParams(newParams);
      populateFromDiagnostic(diagnostic, newParams);
    } catch (err) {
      setGlobalError(
        err instanceof Error ? err.message : 'Failed to load diagnostic'
      );
    }
  };

  const populateFromDiagnostic = (
    diagnostic: RawDiagnostic,
    currentParams: QueryParams
  ) => {
    setDiagnosticsId(diagnostic.id);
    setResponseTimeMs(diagnostic.response_time_ms);

    if (diagnostic.plan) {
      const tree = convertQueryPlanToTree(diagnostic.plan);
      setPlanTree(tree);
      setPlanJson(JSON.stringify(diagnostic.plan, null, 2));

      const blocks = extractBlocksFromPlan(diagnostic.plan);
      const startTime = parseTimeForStats(currentParams.startTime);
      const endTime = parseTimeForStats(currentParams.endTime);
      const stats = buildMetadataStats(blocks, startTime, endTime);
      setMetadataStats(stats);
    }

    if (diagnostic.execution) {
      const tree = convertExecutionNodeToTree(diagnostic.execution);
      setExecutionTree(tree);
    }
  };

  const handleSubmit = useCallback(async () => {
    if (!params.tenantId) {
      setError('Tenant ID is required');
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const result = await executeQuery(params);
      if (result.diagnosticsId) {
        window.location.href = `/query-diagnostics?load=${result.diagnosticsId}&tenant=${params.tenantId}`;
      } else {
        setError('Query succeeded but no diagnostics ID was returned');
      }
    } catch (err) {
      setError(
        'Query failed: ' + (err instanceof Error ? err.message : String(err))
      );
    } finally {
      setIsSubmitting(false);
    }
  }, [params]);

  return (
    <main>
      <div className="container mt-5">
        <Header
          title="Query Diagnostics"
          subtitle="Debug V2 query execution by running queries and viewing execution traces"
          showNewQueryLink={true}
          showStoredDiagnosticsLink={true}
        />

        {globalError && (
          <div className="alert alert-danger mt-3" role="alert">
            <strong>Error:</strong> {globalError}
          </div>
        )}

        <div className="row mt-4">
          <div className="col-12 col-lg-5">
            <QueryForm
              tenants={tenants}
              params={params}
              onParamsChange={setParams}
              onSubmit={handleSubmit}
              isSubmitting={isSubmitting}
              error={error}
            />
          </div>

          <div className="col-12 col-lg-7 mt-4 mt-lg-0">
            <QueryPlanViewer
              planTree={planTree}
              planJson={planJson}
              metadataStats={metadataStats}
            />
          </div>
        </div>

        <ExecutionTraceViewer
          executionTree={executionTree}
          responseTimeMs={responseTimeMs}
          diagnosticsId={diagnosticsId}
        />
      </div>
    </main>
  );
}
