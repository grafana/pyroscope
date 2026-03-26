import { useCallback, useEffect, useState } from 'react';
import {
  fetchServices,
  fetchFlamegraph,
  fetchTimeline,
  type Service,
  type FlamegraphData,
} from '@api/client';
export type { Service, FlamegraphData } from '@api/client';

export type ProfileType = string;

export interface QueryParams {
  service: string;
  profileType: ProfileType;
  timeRange: string;
  absoluteRange?: { start: number; end: number };
  tenantID?: string;
}

export interface QueryResult {
  services: Service[];
  servicesLoading: boolean;
  flamegraph: FlamegraphData;
  timeline: number[];
  loading: boolean;
  error: string | null;
  run: () => void;
  execute: (service: string, profileType: string, timeRange: string) => void;
}

function parseTimeRange(range: string): { start: number; end: number } {
  const now = Date.now();
  const m = range.match(/^now-(\d+)([mhd])$/);
  if (!m) return { start: now - 3_600_000, end: now };
  const mult: Record<string, number> = {
    m: 60_000,
    h: 3_600_000,
    d: 86_400_000,
  };
  const durationMs = parseInt(m[1]) * (mult[m[2]] ?? 60_000);
  return { start: now - durationMs, end: now };
}

export function usePyroscopeQuery(params: QueryParams): QueryResult {
  const [services, setServices] = useState<Service[]>([]);
  const [flamegraph, setFlamegraph] = useState<FlamegraphData>({
    names: [],
    levels: [],
  });
  const [timeline, setTimeline] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const [servicesLoading, setServicesLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const { service, profileType, timeRange, absoluteRange, tenantID } = params;

  useEffect(() => {
    setServicesLoading(true);
    const { start, end } = absoluteRange ?? parseTimeRange(timeRange);
    fetchServices(start, end)
      .then((s) => {
        setServices(s);
        setError(null);
      })
      .catch((e: unknown) =>
        setError(e instanceof Error ? e.message : String(e)),
      )
      .finally(() => setServicesLoading(false));
  }, [timeRange, absoluteRange, tenantID]);

  const execute = useCallback(
    (svc: string, pt: string, tr: string) => {
      if (!svc || !pt) return;
      const { start, end } = absoluteRange ?? parseTimeRange(tr);
      const labelSelector = `{service_name="${svc}"}`;
      const rangeSeconds = (end - start) / 1000;
      const step = Math.max(15, Math.ceil(rangeSeconds / 100));
      setLoading(true);
      Promise.all([
        fetchFlamegraph({ profileTypeID: pt, labelSelector, start, end }),
        fetchTimeline({ profileTypeID: pt, labelSelector, start, end, step }),
      ])
        .then(([fg, tl]) => {
          setFlamegraph(fg);
          setTimeline(tl);
          setError(null);
        })
        .catch((e: unknown) =>
          setError(e instanceof Error ? e.message : String(e)),
        )
        .finally(() => setLoading(false));
    },
    // tenantID is not read inside the callback but is included to invalidate
    // execute (and run) when the tenant changes, ensuring the run callback is
    // re-created.
    //
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [absoluteRange, tenantID],
  );

  useEffect(() => {
    if (!service || !profileType) return;
    const { start, end } = absoluteRange ?? parseTimeRange(timeRange);
    const labelSelector = `{service_name="${service}"}`;
    const rangeSeconds = (end - start) / 1000;
    const step = Math.max(15, Math.ceil(rangeSeconds / 100));

    setLoading(true);
    Promise.all([
      fetchFlamegraph({
        profileTypeID: profileType,
        labelSelector,
        start,
        end,
      }),
      fetchTimeline({
        profileTypeID: profileType,
        labelSelector,
        start,
        end,
        step,
      }),
    ])
      .then(([fg, tl]) => {
        setFlamegraph(fg);
        setTimeline(tl);
        setError(null);
      })
      .catch((e: unknown) =>
        setError(e instanceof Error ? e.message : String(e)),
      )
      .finally(() => setLoading(false));
  }, [service, profileType, timeRange, absoluteRange, tenantID]);

  const run = useCallback(() => {
    execute(service, profileType, timeRange);
  }, [service, profileType, timeRange, execute]);

  return {
    services,
    servicesLoading,
    flamegraph,
    timeline,
    loading,
    error,
    run,
    execute,
  };
}
