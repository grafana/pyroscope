import { z } from 'zod';

// RawFlamebearerProfile represents the exact FlamebearerProfile it's gotten from the backend
// export interface Profile {
//  version: number;
//
//  metadata: {
//    // Optional fields since adhoc may be missing them
//    // they are added on /render and /render-diff response
//    // https://github.com/pyroscope-io/pyroscope/blob/main/pkg/server/render.go#L114-L131
//    appName?: string;
//    startTime?: number;
//    endTime?: number;
//    query?: string;
//    maxNodes?: number;
//
//    units: 'samples' | 'objects' | 'bytes' | string;
//  };
//
//  flamebearer: {
//    /**
//     * List of names
//     */
//    names: string[];
//    /**
//     * List of level
//     *
//     * This is NOT the same as in the flamebearer
//     * that we receive from the server.
//     * As in there are some transformations required
//     * (see deltaDiffWrapper)
//     */
//    levels: number[][];
//    numTicks: number;
//  };
// }
//
export const FlamebearerSchema = z.object({
  names: z.array(z.string().nonempty()),
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

  // accept the defined units
  // and convert anything else to empty string
  units: z.preprocess((u) => {
    const units = ['samples', 'objects', 'bytes'];
    if (typeof u === 'string') {
      if (units.includes(u)) {
        return u;
      }
    }
    return '';
  }, z.enum(['samples', 'objects', 'bytes', ''])),

  spyName: z
    .enum(['dotnetspy', 'ebpfspy', 'gospy', 'phpspy', 'pyspy', 'rbspy'])
    .or(z.string()),
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
