import { z } from 'zod';

export const units = [
  'samples',
  'objects',
  'goroutines',
  'bytes',
  'lock_samples',
  'lock_nanoseconds',
  'trace_samples',
  'exceptions',
  'nanoseconds',
] as const;

export const UnitsSchema = z.preprocess(
  (u) => units.find((knownUnit) => u === knownUnit) || 'unknown',
  z.enum([...units, 'unknown'])
);

export type UnitsType = (typeof units)[number];
export type Units = z.infer<typeof UnitsSchema>;
