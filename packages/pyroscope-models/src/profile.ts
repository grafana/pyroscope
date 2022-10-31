import { z } from 'zod';
import { SpyNameSchema } from './spyName';

export const FlamebearerSchema = z.object({
  names: z.array(
    z.preprocess((n) => {
      if (!n) {
        return 'unknown';
      }

      return n;
    }, z.string().min(1))
  ),
  levels: z.array(z.array(z.number())),
  numTicks: z.number(),
  maxSelf: z.number(),
});

export type UnitsType = typeof units[number];

export const units = [
  'samples',
  'objects',
  'goroutines',
  'bytes',
  'lock_samples',
  'lock_nanoseconds',
  'trace_samples',
];

export const unitsDescription = {
  objects: 'number of objects in RAM per function',
  goroutines: 'number of goroutines',
  bytes: 'amount of RAM per function',
  samples: 'CPU time per function',
  lock_nanoseconds: 'time spent waiting on locks per function',
  lock_samples: 'number of contended locks per function',
  trace_samples: 'aggregated span duration',
  '': '',
};

// accept the defined units
// and convert anything else to empty string
export const UnitsSchema = z.preprocess((u) => {
  if (typeof u === 'string') {
    if (units.includes(u)) {
      return u;
    }
  }
  return '';
}, z.enum(['samples', 'objects', 'goroutines', 'bytes', 'lock_samples', 'lock_nanoseconds', 'trace_samples', '']));

export const MetadataSchema = z.object({
  // Optional fields since adhoc may be missing them
  // they are added on /render and /render-diff response
  // https://github.com/pyroscope-io/pyroscope/blob/main/pkg/server/render.go#L114-L131
  appName: z.string().optional(),
  name: z.string().optional(),
  startTime: z.number().optional(),
  endTime: z.number().optional(),
  query: z.string().optional(),
  maxNodes: z.number().optional(),

  format: z.enum(['single', 'double']),
  sampleRate: z.number(),
  spyName: SpyNameSchema,

  units: UnitsSchema,
});

export const FlamebearerProfileSchema = z.object({
  version: z.number().min(1).max(1).default(1),
  flamebearer: FlamebearerSchema,
  metadata: MetadataSchema,

  // TODO make thee dependent on format === 'double'
  leftTicks: z.number().optional(),
  rightTicks: z.number().optional(),
});

export type Profile = z.infer<typeof FlamebearerProfileSchema>;
export type Units = z.infer<typeof UnitsSchema>;
