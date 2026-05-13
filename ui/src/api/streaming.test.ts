import { describe, expect, it } from 'vitest';
import { readEnvelopes, type Envelope } from './streaming';

function buildEnvelope(flags: number, body: string): Uint8Array {
  const payload = new TextEncoder().encode(body);
  const out = new Uint8Array(5 + payload.byteLength);
  const view = new DataView(out.buffer);
  view.setUint8(0, flags);
  view.setUint32(1, payload.byteLength, false);
  out.set(payload, 5);
  return out;
}

function streamOf(chunks: Uint8Array[]): ReadableStream<Uint8Array> {
  let i = 0;
  return new ReadableStream<Uint8Array>({
    pull(controller) {
      if (i < chunks.length) {
        controller.enqueue(chunks[i++]);
      } else {
        controller.close();
      }
    },
  });
}

async function collect(
  stream: ReadableStream<Uint8Array>,
): Promise<{ flags: number; body: string }[]> {
  const out: { flags: number; body: string }[] = [];
  const dec = new TextDecoder();
  for await (const frame of readEnvelopes(stream) as AsyncGenerator<Envelope>) {
    out.push({ flags: frame.flags, body: dec.decode(frame.payload) });
  }
  return out;
}

describe('readEnvelopes', () => {
  it('decodes a single envelope', async () => {
    const frames = await collect(
      streamOf([buildEnvelope(0, '{"progress":{}}')]),
    );
    expect(frames).toEqual([{ flags: 0, body: '{"progress":{}}' }]);
  });

  it('decodes multiple envelopes from one chunk', async () => {
    const chunk = new Uint8Array([
      ...buildEnvelope(0, '{"a":1}'),
      ...buildEnvelope(0, '{"b":2}'),
      ...buildEnvelope(0x02, '{}'),
    ]);
    const frames = await collect(streamOf([chunk]));
    expect(frames).toEqual([
      { flags: 0, body: '{"a":1}' },
      { flags: 0, body: '{"b":2}' },
      { flags: 0x02, body: '{}' },
    ]);
  });

  it('reassembles an envelope split across multiple chunks', async () => {
    const full = buildEnvelope(0, '{"x":"split"}');
    const a = full.subarray(0, 3);
    const b = full.subarray(3, 7);
    const c = full.subarray(7);
    const frames = await collect(streamOf([a, b, c]));
    expect(frames).toEqual([{ flags: 0, body: '{"x":"split"}' }]);
  });

  it('handles an envelope arriving in multiple reads followed by another in the next chunk', async () => {
    const first = buildEnvelope(0, '{"a":1}');
    const second = buildEnvelope(0x02, '{}');
    // Split the first envelope across two chunks; deliver second whole.
    const frames = await collect(
      streamOf([first.subarray(0, 4), first.subarray(4), second]),
    );
    expect(frames).toEqual([
      { flags: 0, body: '{"a":1}' },
      { flags: 0x02, body: '{}' },
    ]);
  });

  it('yields nothing for an empty stream', async () => {
    const frames = await collect(streamOf([]));
    expect(frames).toEqual([]);
  });

  it('exposes the end-stream flag bit', async () => {
    const frames = await collect(
      streamOf([
        buildEnvelope(0, '{"progress":{}}'),
        buildEnvelope(0x02, '{"error":{"code":"unimplemented"}}'),
      ]),
    );
    expect(frames[0].flags & 0x02).toBe(0);
    expect(frames[1].flags & 0x02).toBe(0x02);
  });
});
