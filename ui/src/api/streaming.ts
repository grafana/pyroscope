// Connect server-streaming over fetch, JSON codec. No external deps.
//
// Wire format (Connect protocol, JSON):
//   POST /<service>/<method>
//     Content-Type: application/connect+json
//     Connect-Protocol-Version: 1
//     Body: one input envelope (5-byte prefix + JSON request).
//   Response: stream of envelopes; the last has flags & 0x02 (end-stream).
//
// Envelope:
//   1 byte  flags (0x01 = compressed, 0x02 = end-stream)
//   4 bytes big-endian uint32 message length
//   N bytes JSON payload
//
// End-stream JSON: {} on success, {"error":{"code":"...","message":"..."}} on
// error.

import { getBasePath, getOrgID } from './client';
import type { FlamegraphData, Point } from './client';

const ENVELOPE_PREFIX_LEN = 5;
const FLAG_END_STREAM = 0x02;

export interface QueryProgress {
  bytesTotalEstimate: number;
  bytesDone: number;
  etaUnixMs: number;
}

export interface FlamegraphProgress extends QueryProgress {
  flamegraph: FlamegraphData | null;
}

export interface SeriesProgress extends QueryProgress {
  series: Point[];
}

export interface StreamCallbacks<P, R> {
  onProgress: (p: P) => void;
  onResult: (r: R) => void;
  onError: (e: Error) => void;
  // Invoked when the server does not implement the streaming RPC (HTTP 404/501
  // or end-stream error code "unimplemented"). The caller should fall back to
  // the unary path.
  onUnimplemented: () => void;
}

export interface FlamegraphParams {
  profileTypeID: string;
  labelSelector: string;
  start: number;
  end: number;
}

export interface SeriesParams extends FlamegraphParams {
  step: number;
}

export function streamFlamegraph(
  params: FlamegraphParams,
  callbacks: StreamCallbacks<FlamegraphProgress, FlamegraphData>,
  signal: AbortSignal,
): void {
  void runStream(
    '/querier.v1.QuerierStreamService/SelectMergeStacktracesStream',
    params,
    callbacks,
    parseFlamegraphEvent,
    signal,
  );
}

export function streamSeries(
  params: SeriesParams,
  callbacks: StreamCallbacks<SeriesProgress, Point[]>,
  signal: AbortSignal,
): void {
  void runStream(
    '/querier.v1.QuerierStreamService/SelectSeriesStream',
    params,
    callbacks,
    parseSeriesEvent,
    signal,
  );
}

type EventResult<P, R> =
  | { kind: 'progress'; value: P }
  | { kind: 'result'; value: R }
  | { kind: 'ignore' };

function parseFlamegraphEvent(
  msg: unknown,
): EventResult<FlamegraphProgress, FlamegraphData> {
  if (!isRecord(msg)) return { kind: 'ignore' };
  if (isRecord(msg.progress)) {
    const p = msg.progress;
    return {
      kind: 'progress',
      value: {
        bytesTotalEstimate: toNum(p.bytesTotalEstimate),
        bytesDone: toNum(p.bytesDone),
        etaUnixMs: toNum(p.etaUnixMs),
        flamegraph: extractFlamegraph(p.flamegraph),
      },
    };
  }
  if ('flamegraph' in msg) {
    return {
      kind: 'result',
      value: extractFlamegraph(msg.flamegraph) ?? { names: [], levels: [] },
    };
  }
  return { kind: 'ignore' };
}

function parseSeriesEvent(
  msg: unknown,
): EventResult<SeriesProgress, Point[]> {
  if (!isRecord(msg)) return { kind: 'ignore' };
  if (isRecord(msg.progress)) {
    const p = msg.progress;
    return {
      kind: 'progress',
      value: {
        bytesTotalEstimate: toNum(p.bytesTotalEstimate),
        bytesDone: toNum(p.bytesDone),
        etaUnixMs: toNum(p.etaUnixMs),
        series: extractSeriesPoints(p.series),
      },
    };
  }
  if (isRecord(msg.result)) {
    return {
      kind: 'result',
      value: extractSeriesPoints(msg.result.series),
    };
  }
  return { kind: 'ignore' };
}

function extractFlamegraph(v: unknown): FlamegraphData | null {
  if (!isRecord(v)) return null;
  const names = Array.isArray(v.names) ? (v.names as string[]) : [];
  const levels = Array.isArray(v.levels)
    ? (v.levels as { values: string[] }[])
    : [];
  if (names.length === 0 && levels.length === 0) return null;
  return { names, levels };
}

function extractSeriesPoints(v: unknown): Point[] {
  if (!Array.isArray(v) || v.length === 0) return [];
  const first = v[0];
  if (!isRecord(first) || !Array.isArray(first.points)) return [];
  return first.points as Point[];
}

function toNum(v: unknown): number {
  if (typeof v === 'number') return v;
  if (typeof v === 'string') return Number(v);
  return 0;
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

async function runStream<P, R>(
  path: string,
  request: unknown,
  callbacks: StreamCallbacks<P, R>,
  parseEvent: (msg: unknown) => EventResult<P, R>,
  signal: AbortSignal,
): Promise<void> {
  let res: Response;
  try {
    res = await fetch(getBasePath() + path, {
      method: 'POST',
      signal,
      headers: buildHeaders(),
      body: encodeEnvelope(0, JSON.stringify(request)),
    });
  } catch (e) {
    if (signal.aborted) return;
    callbacks.onError(toError(e));
    return;
  }

  // Route not registered (V1-only deployment or older backend) → caller falls
  // back to the unary path.
  if (res.status === 404 || res.status === 501) {
    callbacks.onUnimplemented();
    return;
  }
  if (!res.ok || !res.body) {
    callbacks.onError(new Error(`${path} ${res.status} ${res.statusText}`));
    return;
  }

  const decoder = new TextDecoder();
  try {
    for await (const frame of readEnvelopes(res.body)) {
      if (frame.flags & FLAG_END_STREAM) {
        handleEndStream(frame.payload, decoder, callbacks);
        return;
      }
      let msg: unknown;
      try {
        msg = JSON.parse(decoder.decode(frame.payload));
      } catch (e) {
        callbacks.onError(
          new Error(`stream JSON parse: ${(e as Error).message}`),
        );
        return;
      }
      const ev = parseEvent(msg);
      if (ev.kind === 'progress') callbacks.onProgress(ev.value);
      else if (ev.kind === 'result') callbacks.onResult(ev.value);
    }
  } catch (e) {
    if (signal.aborted) return;
    callbacks.onError(toError(e));
  }
}

function handleEndStream<P, R>(
  payload: Uint8Array,
  decoder: TextDecoder,
  callbacks: StreamCallbacks<P, R>,
): void {
  if (payload.byteLength === 0) return;
  let body: unknown;
  try {
    body = JSON.parse(decoder.decode(payload));
  } catch {
    return;
  }
  if (!isRecord(body) || !isRecord(body.error)) return;
  const code = typeof body.error.code === 'string' ? body.error.code : '';
  if (code === 'unimplemented') {
    callbacks.onUnimplemented();
    return;
  }
  const message =
    typeof body.error.message === 'string'
      ? body.error.message
      : `stream error: ${code || 'unknown'}`;
  callbacks.onError(new Error(message));
}

function buildHeaders(): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/connect+json',
    'Connect-Protocol-Version': '1',
  };
  const id = getOrgID();
  if (id) headers['X-Scope-OrgID'] = id;
  return headers;
}

function encodeEnvelope(flags: number, body: string): Uint8Array<ArrayBuffer> {
  const payload = new TextEncoder().encode(body);
  const buffer = new ArrayBuffer(ENVELOPE_PREFIX_LEN + payload.byteLength);
  const out = new Uint8Array(buffer);
  const view = new DataView(buffer);
  view.setUint8(0, flags);
  view.setUint32(1, payload.byteLength, false);
  out.set(payload, ENVELOPE_PREFIX_LEN);
  return out;
}

function toError(e: unknown): Error {
  return e instanceof Error ? e : new Error(String(e));
}

export interface Envelope {
  flags: number;
  payload: Uint8Array;
}

// readEnvelopes pulls length-prefixed frames out of a ReadableStream. Exported
// for unit testing; runStream is the only production caller.
export async function* readEnvelopes(
  stream: ReadableStream<Uint8Array>,
): AsyncGenerator<Envelope, void, unknown> {
  const reader = stream.getReader();
  try {
    let buf: Uint8Array = new Uint8Array(0);
    for (;;) {
      const { value, done } = await reader.read();
      if (value && value.byteLength > 0) buf = concat(buf, value);
      while (buf.byteLength >= ENVELOPE_PREFIX_LEN) {
        const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
        const flags = view.getUint8(0);
        const len = view.getUint32(1, false);
        if (buf.byteLength < ENVELOPE_PREFIX_LEN + len) break;
        const payload = buf.subarray(
          ENVELOPE_PREFIX_LEN,
          ENVELOPE_PREFIX_LEN + len,
        );
        const next = buf.subarray(ENVELOPE_PREFIX_LEN + len);
        yield { flags, payload };
        buf = next;
      }
      if (done) return;
    }
  } finally {
    try {
      reader.releaseLock();
    } catch {
      // releaseLock can throw if the reader is already detached; ignore.
    }
  }
}

function concat(a: Uint8Array, b: Uint8Array): Uint8Array {
  if (a.byteLength === 0) return b;
  const out = new Uint8Array(a.byteLength + b.byteLength);
  out.set(a, 0);
  out.set(b, a.byteLength);
  return out;
}
