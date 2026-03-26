import registry from './profileTypes.json';

export interface Service {
  name: string;
  profileTypes: string[];
}

export interface FlamegraphData {
  names: string[];
  levels: { values: string[] }[];
}

// Reads the <base href> injected by the Go server. Returns a path prefix like
// "/uscentral" (no trailing slash), or "" when running at the root or in dev.
function getBasePath(): string {
  const base = document.querySelector('base') as HTMLBaseElement | null;
  if (!base) return '';
  const href = base.getAttribute('href') ?? '';
  // When the Go template hasn't been rendered (dev mode) the literal string
  // "{{ .BaseURL }}" appears; treat it as no prefix.
  if (href.includes('{{')) return '';
  return href === '/' ? '' : href.replace(/\/$/, '');
}

let orgID = '';

export function setOrgID(id: string) {
  orgID = id;
}

export async function checkMultitenancy(): Promise<
  'single_tenant' | 'multi_tenant' | 'error'
> {
  try {
    const res = await fetch(
      `${getBasePath()}/querier.v1.QuerierService/LabelNames`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Scope-OrgID': '',
        },
        body: JSON.stringify({ matchers: [] }),
      },
    );
    if (res.ok) return 'single_tenant';
    if (res.status === 401) return 'multi_tenant';
    return 'error';
  } catch {
    return 'error';
  }
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  if (orgID) headers['X-Scope-OrgID'] = orgID;
  const res = await fetch(getBasePath() + path, {
    method: 'POST',
    headers,
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

export async function fetchServices(
  from: number,
  until: number,
): Promise<Service[]> {
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

  return points.map((p) => p.value);
}

function parseProfileTypeId(
  id: string,
): { group: string; type: string; unit: string } | null {
  const parts = id.split(':');
  // format: <name>:<type>:<unit>:<period_type>:<period_unit>
  if (parts.length !== 5) return null;
  return {
    group: parts[0] ?? id,
    type: parts[1] ?? id,
    unit: parts[2] ?? 'count',
  };
}

export function profileTypeLabel(id: string): string {
  const entry = (registry as Record<string, { type: string }>)[id];
  return entry?.type ?? parseProfileTypeId(id)?.type ?? id;
}

export function profileTypeGroup(id: string): string {
  const entry = (registry as Record<string, { group: string }>)[id];
  return entry?.group ?? parseProfileTypeId(id)?.group ?? id;
}

export function profileTypeUnit(id: string): string {
  const entry = (registry as Record<string, { unit: string }>)[id];
  return entry?.unit ?? parseProfileTypeId(id)?.unit ?? 'count';
}

export function profileTypeRateLabel(id: string): string {
  switch (profileTypeUnit(id)) {
    case 'ns':
      return 'cpu cores';
    case 'bytes':
      return 'bytes / sec';
    default:
      return 'samples / sec';
  }
}

const PINNED_GROUPS = ['process_cpu', 'memory'];

// Returns profile type IDs interleaved with section headers for each group.
// Groups are ordered: pinned groups first, then alphabetical.
// Profile types within each group are sorted alphabetically by type label.
export function sortProfileTypes(
  ids: string[],
): Array<string | { section: string }> {
  const byGroup = new Map<string, string[]>();
  for (const id of ids) {
    const group = profileTypeGroup(id);
    if (!byGroup.has(group)) byGroup.set(group, []);
    byGroup.get(group)!.push(id);
  }

  const groupNames = Array.from(byGroup.keys()).sort((a, b) => {
    const ai = PINNED_GROUPS.indexOf(a),
      bi = PINNED_GROUPS.indexOf(b);
    if (ai !== -1 && bi !== -1) return ai - bi;
    if (ai !== -1) return -1;
    if (bi !== -1) return 1;
    return a.localeCompare(b);
  });

  const result: Array<string | { section: string }> = [];
  for (const group of groupNames) {
    const members = byGroup
      .get(group)!
      .sort((a, b) => profileTypeLabel(a).localeCompare(profileTypeLabel(b)));
    result.push({ section: group });
    result.push(...members);
  }
  return result;
}
