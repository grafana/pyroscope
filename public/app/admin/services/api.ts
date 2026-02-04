import type {
  DiagnosticSummary,
  QueryMethod,
  QueryParams,
  RawDiagnostic,
} from '../types';
import { getBasePath } from '../utils';

export async function fetchTenants(): Promise<string[]> {
  const basePath = getBasePath();
  const response = await fetch(`${basePath}/query-diagnostics/api/tenants`);
  if (!response.ok) {
    throw new Error(`Failed to fetch tenants: ${response.status}`);
  }
  return response.json();
}

export async function fetchProfileTypes(
  tenantId: string,
  startMs: number,
  endMs: number
): Promise<string[]> {
  const basePath = getBasePath();
  const response = await fetch(
    `${basePath}/querier.v1.QuerierService/ProfileTypes`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Scope-OrgID': tenantId,
      },
      body: JSON.stringify({ start: startMs, end: endMs }),
    }
  );
  if (!response.ok) {
    throw new Error(`Failed to fetch profile types: ${response.status}`);
  }
  const data = await response.json();
  return (data.profileTypes || []).map((pt: { ID: string }) => pt.ID);
}

export async function listDiagnostics(
  tenant: string
): Promise<DiagnosticSummary[]> {
  const basePath = getBasePath();
  const response = await fetch(
    `${basePath}/query-diagnostics/api/diagnostics?tenant=${encodeURIComponent(
      tenant
    )}`
  );
  if (!response.ok) {
    throw new Error(`Failed to list diagnostics: ${response.status}`);
  }
  return response.json();
}

export async function loadDiagnostic(
  tenant: string,
  id: string
): Promise<RawDiagnostic> {
  const basePath = getBasePath();
  const response = await fetch(
    `${basePath}/query-diagnostics/api/diagnostics/${encodeURIComponent(
      id
    )}?tenant=${encodeURIComponent(tenant)}`
  );
  if (!response.ok) {
    throw new Error(`Failed to load diagnostic: ${response.status}`);
  }
  return response.json();
}

function parseTime(timeStr: string): number {
  if (!timeStr) {
    return 0;
  }
  const now = Date.now();
  const match = timeStr.match(/^now(-(\d+)([smhd]))?$/);
  if (match) {
    if (!match[1]) {
      return now;
    }
    const value = parseInt(match[2], 10);
    const unit = match[3];
    const multipliers: Record<string, number> = {
      s: 1000,
      m: 60000,
      h: 3600000,
      d: 86400000,
    };
    return now - value * (multipliers[unit] || 0);
  }
  const d = new Date(timeStr);
  return isNaN(d.getTime()) ? 0 : d.getTime();
}

function calculateDefaultStep(startMs: number, endMs: number): number {
  const minStep = 15;
  const targetDataPoints = 100;
  const rangeSeconds = (endMs - startMs) / 1000;
  const calculatedStep = rangeSeconds / targetDataPoints;
  return Math.max(minStep, Math.ceil(calculatedStep));
}

function buildRequestBody(
  method: QueryMethod,
  params: QueryParams
): Record<string, unknown> {
  const startMs = parseTime(params.startTime);
  const endMs = parseTime(params.endTime);

  switch (method) {
    case 'SelectMergeStacktraces':
    case 'SelectMergeProfile': {
      const body: Record<string, unknown> = {
        start: startMs,
        end: endMs,
        labelSelector: params.labelSelector,
        profileTypeID: params.profileTypeId,
      };
      if (params.maxNodes) {
        body.maxNodes = parseInt(params.maxNodes, 10);
      }
      if (params.format) {
        body.format = params.format;
      }
      return body;
    }
    case 'SelectMergeSpanProfile': {
      const body: Record<string, unknown> = {
        start: startMs,
        end: endMs,
        labelSelector: params.labelSelector,
        profileTypeID: params.profileTypeId,
      };
      if (params.maxNodes) {
        body.maxNodes = parseInt(params.maxNodes, 10);
      }
      if (params.format) {
        body.format = params.format;
      }
      if (params.spanSelector) {
        body.spanSelector = params.spanSelector
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
      }
      return body;
    }
    case 'SelectSeries': {
      const body: Record<string, unknown> = {
        start: startMs,
        end: endMs,
        labelSelector: params.labelSelector,
        profileTypeID: params.profileTypeId,
        step: params.step
          ? parseInt(params.step, 10)
          : calculateDefaultStep(startMs, endMs),
      };
      if (params.groupBy) {
        body.groupBy = params.groupBy
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
      }
      if (params.aggregation === 'sum') {
        body.aggregation = 'TIME_SERIES_AGGREGATION_TYPE_SUM';
      } else if (params.aggregation === 'avg') {
        body.aggregation = 'TIME_SERIES_AGGREGATION_TYPE_AVERAGE';
      }
      if (params.limit) {
        body.limit = parseInt(params.limit, 10);
      }
      if (params.exemplarType) {
        body.exemplarType = params.exemplarType;
      }
      return body;
    }
    case 'SelectHeatmap': {
      const body: Record<string, unknown> = {
        start: startMs,
        end: endMs,
        labelSelector: params.labelSelector,
        profileTypeID: params.profileTypeId,
        queryType: params.heatmapQueryType,
        step: params.step
          ? parseInt(params.step, 10)
          : calculateDefaultStep(startMs, endMs),
      };
      if (params.groupBy) {
        body.groupBy = params.groupBy
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
      }
      if (params.limit) {
        body.limit = parseInt(params.limit, 10);
      }
      if (params.exemplarType) {
        body.exemplarType = params.exemplarType;
      }
      return body;
    }
    case 'Diff': {
      const leftStartMs = parseTime(params.diffLeftStart);
      const leftEndMs = parseTime(params.diffLeftEnd);
      const rightStartMs = parseTime(params.diffRightStart);
      const rightEndMs = parseTime(params.diffRightEnd);
      return {
        left: {
          start: leftStartMs,
          end: leftEndMs,
          labelSelector: params.diffLeftSelector,
          profileTypeID: params.diffLeftProfileType,
        },
        right: {
          start: rightStartMs,
          end: rightEndMs,
          labelSelector: params.diffRightSelector,
          profileTypeID: params.diffRightProfileType,
        },
      };
    }
    case 'LabelNames':
      return {
        start: startMs,
        end: endMs,
        matchers: params.labelSelector ? [params.labelSelector] : [],
      };
    case 'LabelValues':
      return {
        start: startMs,
        end: endMs,
        name: params.labelName,
        matchers: params.labelSelector ? [params.labelSelector] : [],
      };
    case 'Series': {
      const body: Record<string, unknown> = {
        start: startMs,
        end: endMs,
        matchers: params.labelSelector ? [params.labelSelector] : [],
      };
      if (params.labelNames) {
        body.labelNames = params.labelNames
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
      }
      return body;
    }
    case 'ProfileTypes':
      return { start: startMs, end: endMs };
    default:
      return {};
  }
}

export interface ExecuteQueryResult {
  diagnosticsId: string | null;
}

export async function executeQuery(
  params: QueryParams
): Promise<ExecuteQueryResult> {
  const basePath = getBasePath();
  const endpoint = `${basePath}/querier.v1.QuerierService/${params.method}`;
  const body = buildRequestBody(params.method, params);

  const response = await fetch(endpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Scope-OrgID': params.tenantId,
      'X-Pyroscope-Collect-Diagnostics': 'true',
    },
    body: JSON.stringify(body),
  });

  const diagnosticsId = response.headers.get('X-Pyroscope-Diagnostics-Id');

  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `HTTP ${response.status}`);
  }

  return { diagnosticsId };
}

export async function exportDiagnostic(
  tenant: string,
  id: string
): Promise<Blob> {
  const basePath = getBasePath();
  const response = await fetch(
    `${basePath}/query-diagnostics/api/export/${encodeURIComponent(
      id
    )}?tenant=${encodeURIComponent(tenant)}`
  );
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Failed to export diagnostic: ${response.status}`);
  }
  return response.blob();
}

export async function importDiagnostic(
  tenant: string,
  file: File
): Promise<{ id: string }> {
  const basePath = getBasePath();
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch(
    `${basePath}/query-diagnostics/api/import?tenant=${encodeURIComponent(
      tenant
    )}`,
    {
      method: 'POST',
      body: formData,
    }
  );
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Failed to import diagnostic: ${response.status}`);
  }
  return response.json();
}
