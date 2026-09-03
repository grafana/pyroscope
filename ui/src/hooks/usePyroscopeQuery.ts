import { useCallback, useEffect, useState } from 'react';
import {
  fetchServices,
  fetchFlamegraph,
  fetchTimeline,
  type Service,
  type FlamegraphData,
  type Point,
} from '@api/client';
export type { Service, FlamegraphData } from '@api/client';

export type ProfileType = string;

export interface QueryParams {
  service: string;
  profileType: ProfileType;
  labelSelector?: string;
  timeRange: string;
  absoluteRange?: { start: number; end: number };
  tenantID?: string;
}

export interface QueryResult {
  services: Service[];
  servicesLoading: boolean;
  flamegraph: FlamegraphData;
  timeline: Point[];
  loading: boolean;
  error: string | null;
  run: () => void;
}

export function parseTimeRange(range: string): { start: number; end: number } {
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
  const [timeline, setTimeline] = useState<Point[]>([]);
  const [loading, setLoading] = useState(false);
  const [servicesLoading, setServicesLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const {
    service,
    profileType,
    labelSelector: paramSelector,
    timeRange,
    absoluteRange,
    tenantID,
  } = params;

  useEffect(() => {
    async function loadServices() {
      setServicesLoading(true);
      const { start, end } = absoluteRange ?? parseTimeRange(timeRange);
      try {
        setServices(await fetchServices(start, end));
        setError(null);
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setServicesLoading(false);
      }
    }
    loadServices();
  }, [timeRange, absoluteRange, tenantID]);

  useEffect(() => {
    if (!service || !profileType) return;

    async function loadProfile() {
      const { start, end } = absoluteRange ?? parseTimeRange(timeRange);
      const labelSelector = paramSelector ?? `{service_name="${service}"}`;
      const rangeSeconds = (end - start) / 1000;
      const step = Math.max(15, Math.ceil(rangeSeconds / 100));

      setLoading(true);
      try {
        const [fg, tl] = await Promise.all([
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
        ]);
        setFlamegraph(fg);
        setTimeline(tl);
        setError(null);
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setLoading(false);
      }
    }
    loadProfile();
  }, [service, profileType, paramSelector, timeRange, absoluteRange, tenantID]);

  const run = useCallback(() => {
    if (!service || !profileType) return;
    const { start, end } = absoluteRange ?? parseTimeRange(timeRange);
    const labelSelector = paramSelector ?? `{service_name="${service}"}`;
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [service, profileType, paramSelector, timeRange, absoluteRange, tenantID]);

  return {
    services,
    servicesLoading,
    flamegraph,
    timeline,
    loading,
    error,
    run,
  };
}
