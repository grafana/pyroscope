import { useCallback, useMemo, useState } from 'react';

export type ProfileType = 'cpu' | 'memory' | 'goroutine' | 'mutex' | 'block';

export interface Service {
  name: string;
  profileTypes: ProfileType[];
}

export type Frame = { name: string; start: number; width: number };

export interface QueryParams {
  service: string;
  profileType: ProfileType;
  timeRange: string;
  filters: Record<string, string>;
}

export interface QueryResult {
  services: Service[];
  labels: Record<string, string[]>;
  flamegraph: Frame[][];
  timeline: number[];
  run: () => void;
}

// Mock data — replace each constant with an API call when wiring up the backend.

const MOCK_SERVICES: Service[] = [
  {
    name: 'api-server',
    profileTypes: ['cpu', 'memory', 'goroutine', 'mutex', 'block'],
  },
  { name: 'frontend', profileTypes: ['cpu', 'memory'] },
  { name: 'worker', profileTypes: ['cpu', 'memory', 'goroutine'] },
  { name: 'database', profileTypes: ['cpu', 'memory', 'goroutine', 'mutex'] },
  { name: 'cache', profileTypes: ['cpu', 'memory'] },
];

const MOCK_LABELS: Record<string, Record<string, string[]>> = {
  'api-server': {
    env: ['production', 'staging', 'dev'],
    region: ['us-east-1', 'eu-west-1', 'ap-southeast-1'],
    version: ['v2.1.4', 'v2.1.3', 'v2.0.0'],
  },
  frontend: {
    env: ['production', 'staging'],
    host: ['web-01', 'web-02', 'web-03'],
  },
  worker: {
    env: ['production', 'staging'],
    queue: ['default', 'high-priority', 'batch'],
  },
  database: {
    env: ['production', 'staging'],
    role: ['primary', 'replica-1', 'replica-2'],
  },
  cache: {
    env: ['production', 'staging'],
    node: ['cache-0', 'cache-1', 'cache-2'],
  },
};

const MOCK_FLAMEGRAPH: Frame[][] = [
  [{ name: 'all', start: 0, width: 100 }],
  [
    { name: 'net/http.(*Server).Serve', start: 0, width: 62 },
    { name: 'worker.(*Pool).Start', start: 62, width: 30 },
    { name: 'runtime.gcBgMarkWorker', start: 92, width: 8 },
  ],
  [
    { name: 'net/http.(*conn).serve', start: 0, width: 61 },
    { name: 'worker.(*Pool).processJob', start: 62, width: 28 },
    { name: 'runtime.gcBgMarkWorker.func1', start: 92, width: 8 },
  ],
  [
    { name: 'handler.ServeHTTP', start: 0, width: 48 },
    { name: 'net.(*conn).Read', start: 48, width: 13 },
    { name: 'compute.Hash', start: 62, width: 18 },
    { name: 'storage.Write', start: 80, width: 10 },
    { name: 'runtime.gcDrain', start: 92, width: 8 },
  ],
  [
    { name: 'db.(*DB).Query', start: 0, width: 28 },
    { name: 'json.Marshal', start: 28, width: 12 },
    { name: 'cache.(*Client).Get', start: 40, width: 8 },
    { name: 'net.(*netFD).Read', start: 48, width: 13 },
    { name: 'crypto/sha256.Sum256', start: 62, width: 17 },
    { name: 'compress.(*Writer).Write', start: 80, width: 9 },
    { name: 'runtime.greyobject', start: 92, width: 8 },
  ],
  [
    { name: 'sql.(*Stmt).QueryContext', start: 0, width: 26 },
    { name: 'reflect.Value.MapIndex', start: 28, width: 11 },
    { name: 'sync.(*Mutex).Lock', start: 40, width: 8 },
    { name: 'syscall.Read', start: 48, width: 12 },
    { name: 'hash.(*digest).Write', start: 62, width: 16 },
    { name: 'compress.(*compressor).deflate', start: 80, width: 9 },
    { name: 'runtime.typedmemmove', start: 92, width: 7 },
  ],
  [
    { name: 'sql.(*Rows).Next', start: 0, width: 24 },
    { name: 'encoding/json.marshalValue', start: 28, width: 11 },
    { name: 'sync.(*Mutex).lockSlow', start: 40, width: 7 },
    { name: 'syscall.Syscall', start: 48, width: 12 },
    { name: 'sha256.block', start: 62, width: 15 },
    { name: 'zlib.(*Writer).Write', start: 80, width: 9 },
    { name: 'runtime.memmove', start: 92, width: 7 },
  ],
];

function pseudoRand(seed: number) {
  let s = (seed * 1000 + 7) >>> 0;
  return () => {
    s = (Math.imul(1664525, s) + 1013904223) >>> 0;
    return s / 0x100000000;
  };
}

function generateTimeline(seed: number, n = 80): number[] {
  const r = pseudoRand(seed);
  const raw = Array.from({ length: n }, (_, i) => {
    const slow = 0.3 + 0.2 * Math.sin(i * 0.13);
    const spike = i > 25 && i < 40 ? 0.55 * Math.sin((i - 25) * 0.38) : 0;
    return Math.max(0, slow + spike + (r() - 0.5) * 0.18);
  });
  const mx = Math.max(...raw);
  return raw.map((v) => v / mx);
}

export function usePyroscopeQuery(params: QueryParams): QueryResult {
  const [runCount, setRunCount] = useState(0);

  const timeline = useMemo(() => generateTimeline(runCount), [runCount]);
  const run = useCallback(() => setRunCount((c) => c + 1), []);

  return {
    services: MOCK_SERVICES,
    labels: MOCK_LABELS[params.service] ?? {},
    flamegraph: MOCK_FLAMEGRAPH,
    timeline,
    run,
  };
}
