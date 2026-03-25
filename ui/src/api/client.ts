import registry from './profileTypes.json';

export interface Service {
  name: string;
  profileTypes: string[];
}

export interface FlamegraphData {
  names: string[];
  levels: { values: string[] }[];
}

const ORG_ID = 'anonymous';

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Scope-OrgID': ORG_ID,
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${path} ${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

interface LabelSet {
  labels: { name: string; value: string }[];
}

interface SeriesResponse {
  labelsSet: LabelSet[];
}

export async function fetchServices(from: number, until: number): Promise<Service[]> {
  const data = await post<SeriesResponse>('/querier.v1.QuerierService/Series', {
    matchers: [],
    labelNames: ['service_name', '__profile_type__'],
    start: from,
    end: until,
  });

  const map = new Map<string, Set<string>>();
  for (const { labels } of data.labelsSet ?? []) {
    let serviceName = '';
    let profileType = '';
    for (const { name, value } of labels) {
      if (name === 'service_name') serviceName = value;
      if (name === '__profile_type__') profileType = value;
    }
    if (!serviceName || !profileType) continue;
    if (!map.has(serviceName)) map.set(serviceName, new Set());
    map.get(serviceName)!.add(profileType);
  }

  return Array.from(map.entries()).map(([name, pts]) => ({
    name,
    profileTypes: Array.from(pts),
  }));
}

interface FlamegraphResponse {
  flamegraph: {
    names: string[];
    levels: { values: string[] }[];
  };
}

export async function fetchFlamegraph(params: {
  profileTypeID: string;
  labelSelector: string;
  start: number;
  end: number;
}): Promise<FlamegraphData> {
  const data = await post<FlamegraphResponse>(
    '/querier.v1.QuerierService/SelectMergeStacktraces',
    params,
  );
  const { names, levels } = data.flamegraph ?? { names: [], levels: [] };
  return { names, levels };
}

interface Point {
  value: number;
  timestamp: number;
}

interface SelectSeriesResponse {
  series: { points: Point[] }[];
}

export async function fetchTimeline(params: {
  profileTypeID: string;
  labelSelector: string;
  start: number;
  end: number;
  step: number;
}): Promise<number[]> {
  const data = await post<SelectSeriesResponse>(
    '/querier.v1.QuerierService/SelectSeries',
    params,
  );

  const points = data.series?.[0]?.points ?? [];
  if (!points.length) return [];

  const values = points.map((p) => p.value);
  const max = Math.max(...values);
  if (max === 0) return values.map(() => 0);
  return values.map((v) => v / max);
}

export function profileTypeLabel(id: string): string {
  return (registry as Record<string, { type: string }>)[id]?.type ?? id;
}

export function profileTypeUnit(id: string): string {
  return (registry as Record<string, { unit: string }>)[id]?.unit ?? 'count';
}
