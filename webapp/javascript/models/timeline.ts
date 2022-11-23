import { z } from 'zod';

// Since the backend may return 'null': https://github.com/pyroscope-io/pyroscope/issues/930
// We create an empty object so that the defaults kick in
export const TimelineSchema = z.preprocess(
  (val: unknown) => {
    return val ?? {};
  },
  z.object({
    startTime: z.number().default(0),
    samples: z.array(z.number()).default([]),
    durationDelta: z.number().default(0),
  })
);

export type Timeline = z.infer<typeof TimelineSchema>;
