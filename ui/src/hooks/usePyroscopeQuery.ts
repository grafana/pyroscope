import { useCallback, useEffect, useState } from 'react';
import { fetchServices, fetchFlamegraph, fetchTimeline, type Service, type Frame } from '@api/client';
export type { Service, Frame } from '@api/client';

export type ProfileType = string;

export interface QueryParams {
  service: string;
  profileType: ProfileType;
  timeRange: string;
}

export interface QueryResult {
  services: Service[];
  servicesLoading: boolean;
  flamegraph: Frame[][];
  timeline: number[];
  loading: boolean;
  error: string | null;
  run: () => void;
}

function parseTimeRange(range: string): { start: number; end: number } {
  const now = Date.now();
  const m = range.match(/^now-(\d+)([mhd])$/);
  if (!m) return { start: now - 3_600_000, end: now };
  const mult: Record<string, number> = { m: 60_000, h: 3_600_000, d: 86_400_000 };
  const durationMs = parseInt(m[1]) * (mult[m[2]] ?? 60_000);
  return { start: now - durationMs, end: now };
}

export function usePyroscopeQuery(params: QueryParams): QueryResult {
  const [services, setServices] = useState<Service[]>([]);
  const [flamegraph, setFlamegraph] = useState<Frame[][]>([]);
  const [timeline, setTimeline] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const [servicesLoading, setServicesLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { timeRange } = params;

  useEffect(() => {
    setServicesLoading(true);
    const { start, end } = parseTimeRange(timeRange);
    fetchServices(start, end)
      .then((s) => { setServices(s); setError(null); })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setServicesLoading(false));
  }, [timeRange]);

  const run = useCallback(() => {
    const { service, profileType, timeRange: tr } = params;
    if (!service || !profileType) return;

    const { start, end } = parseTimeRange(tr);
    const labelSelector = `{service_name="${service}"}`;
    const rangeSeconds = (end - start) / 1000;
    const step = Math.max(15, Math.ceil(rangeSeconds / 100));

    setLoading(true);
    Promise.all([
      fetchFlamegraph({ profileTypeID: profileType, labelSelector, start, end }),
      fetchTimeline({ profileTypeID: profileType, labelSelector, start, end, step }),
    ])
      .then(([fg, tl]) => {
        setFlamegraph(fg);
        setTimeline(tl);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [params]);

  return { services, servicesLoading, flamegraph, timeline, loading, error, run };
}
