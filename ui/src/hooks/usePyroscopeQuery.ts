import { useCallback, useEffect, useRef, useState } from 'react';
import {
  fetchServices,
  fetchFlamegraph,
  fetchTimeline,
  type Service,
  type FlamegraphData,
  type Point,
} from '@api/client';
import { streamFlamegraph, streamSeries } from '@api/streaming';
export type { Service, FlamegraphData } from '@api/client';

export type ProfileType = string;

export interface QueryParams {
  service: string;
  profileType: ProfileType;
  timeRange: string;
  absoluteRange?: { start: number; end: number };
  tenantID?: string;
}

export interface QueryProgress {
  bytesTotalEstimate: number;
  bytesDone: number;
  etaUnixMs: number;
}

export interface QueryWindow {
  start: number;
  end: number;
}

export interface QueryResult {
  services: Service[];
  servicesLoading: boolean;
  flamegraph: FlamegraphData;
  timeline: Point[];
  // Resolved absolute window of the in-flight or most recent query. Charts
  // should render their time axis from this so the displayed range matches
  // what was actually queried (rather than drifting against Date.now() on
  // each render).
  queryWindow: QueryWindow;
  progress: {
    flamegraph: QueryProgress | null;
    timeline: QueryProgress | null;
  };
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
  const [timeline, setTimeline] = useState<Point[]>([]);
  const [progress, setProgress] = useState<QueryResult['progress']>({
    flamegraph: null,
    timeline: null,
  });
  const [loading, setLoading] = useState(false);
  const [servicesLoading, setServicesLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [queryWindow, setQueryWindow] = useState<QueryWindow>(() =>
    params.absoluteRange ?? parseTimeRange(params.timeRange),
  );

  const abortRef = useRef<AbortController | null>(null);
  // Monotonic per-run token: callbacks that resolve after a newer run has
  // started must not overwrite state. Critical for the unary fallback path,
  // whose fetch we cannot cancel via signal.
  const runIdRef = useRef(0);

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

  // runQuery only closes over stable setters and refs, so useCallback with an
  // empty dep list is correct.
  const runQuery = useCallback((
    start: number,
    end: number,
    labelSelector: string,
    pt: string,
    step: number,
  ) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    const runId = ++runIdRef.current;
    const isCurrent = () => runId === runIdRef.current;

    setQueryWindow({ start, end });
    setProgress({ flamegraph: null, timeline: null });
    setError(null);
    setLoading(true);

    let flameDone = false;
    let timelineDone = false;
    const finishOne = () => {
      if (!isCurrent()) return;
      if (flameDone && timelineDone) setLoading(false);
    };

    const flameParams = { profileTypeID: pt, labelSelector, start, end };
    streamFlamegraph(
      flameParams,
      {
        onProgress: (p) => {
          if (!isCurrent()) return;
          if (p.flamegraph) setFlamegraph(p.flamegraph);
          setProgress((prev) => ({
            ...prev,
            flamegraph: {
              bytesTotalEstimate: p.bytesTotalEstimate,
              bytesDone: p.bytesDone,
              etaUnixMs: p.etaUnixMs,
            },
          }));
        },
        onResult: (fg) => {
          if (!isCurrent()) return;
          setFlamegraph(fg);
          flameDone = true;
          finishOne();
        },
        onError: (e) => {
          if (!isCurrent()) return;
          setError(e.message);
          flameDone = true;
          finishOne();
        },
        onUnimplemented: () => {
          fetchFlamegraph(flameParams)
            .then((fg) => {
              if (isCurrent()) setFlamegraph(fg);
            })
            .catch((e: unknown) => {
              if (isCurrent())
                setError(e instanceof Error ? e.message : String(e));
            })
            .finally(() => {
              flameDone = true;
              finishOne();
            });
        },
      },
      ctrl.signal,
    );

    const seriesParams = { profileTypeID: pt, labelSelector, start, end, step };
    streamSeries(
      seriesParams,
      {
        onProgress: (p) => {
          if (!isCurrent()) return;
          if (p.series.length > 0) setTimeline(p.series);
          setProgress((prev) => ({
            ...prev,
            timeline: {
              bytesTotalEstimate: p.bytesTotalEstimate,
              bytesDone: p.bytesDone,
              etaUnixMs: p.etaUnixMs,
            },
          }));
        },
        onResult: (pts) => {
          if (!isCurrent()) return;
          setTimeline(pts);
          timelineDone = true;
          finishOne();
        },
        onError: (e) => {
          if (!isCurrent()) return;
          setError(e.message);
          timelineDone = true;
          finishOne();
        },
        onUnimplemented: () => {
          fetchTimeline(seriesParams)
            .then((tl) => {
              if (isCurrent()) setTimeline(tl);
            })
            .catch((e: unknown) => {
              if (isCurrent())
                setError(e instanceof Error ? e.message : String(e));
            })
            .finally(() => {
              timelineDone = true;
              finishOne();
            });
        },
      },
      ctrl.signal,
    );
  }, []);

  const execute = useCallback(
    (svc: string, pt: string, tr: string) => {
      if (!svc || !pt) return;
      const { start, end } = absoluteRange ?? parseTimeRange(tr);
      const labelSelector = `{service_name="${svc}"}`;
      const rangeSeconds = (end - start) / 1000;
      const step = Math.max(15, Math.ceil(rangeSeconds / 100));
      runQuery(start, end, labelSelector, pt, step);
    },
    // tenantID is not read inside the callback but is included to invalidate
    // execute (and run) when the tenant changes, ensuring the run callback is
    // re-created.
    //
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [absoluteRange, tenantID, runQuery],
  );

  useEffect(() => {
    if (!service || !profileType) return;
    const { start, end } = absoluteRange ?? parseTimeRange(timeRange);
    const labelSelector = `{service_name="${service}"}`;
    const rangeSeconds = (end - start) / 1000;
    const step = Math.max(15, Math.ceil(rangeSeconds / 100));
    runQuery(start, end, labelSelector, profileType, step);
    return () => {
      abortRef.current?.abort();
    };
  }, [
    service,
    profileType,
    timeRange,
    absoluteRange,
    tenantID,
    runQuery,
  ]);

  const run = useCallback(() => {
    execute(service, profileType, timeRange);
  }, [service, profileType, timeRange, execute]);

  return {
    services,
    servicesLoading,
    flamegraph,
    timeline,
    queryWindow,
    progress,
    loading,
    error,
    run,
    execute,
  };
}
