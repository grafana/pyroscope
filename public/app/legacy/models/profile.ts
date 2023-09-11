import { z } from 'zod';
import { SpyNameSchema } from './spyName';
import { UnitsSchema } from './units';

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
