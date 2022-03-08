import { z } from 'zod';

export const TimelineSchema = z
  .object({
    startTime: z.number().default(0),
    samples: z.array(z.number()).default([]),
    durationDelta: z.number().default(0),
  })
  .default({});

export type Timeline = z.infer<typeof TimelineSchema>;
