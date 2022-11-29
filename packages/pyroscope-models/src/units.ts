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
];

export const UnitsSchema = z.preprocess((u) => {
  if (typeof u === 'string') {
    if (units.includes(u)) {
      return u;
    }
  }
  return 'unknown';
}, z.enum(['samples', 'objects', 'goroutines', 'bytes', 'lock_samples', 'lock_nanoseconds', 'trace_samples', 'exceptions', 'unknown']));

export type UnitsType = typeof units[number];
export type Units = z.infer<typeof UnitsSchema>;
