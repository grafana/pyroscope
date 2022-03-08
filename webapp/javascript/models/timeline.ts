import { z } from 'zod';

export const TimelineSchema = z.object({
  startTime: z.number(),
  samples: z.array(z.number()),
  durationDelta: z.number(),
});

export type Timeline = z.infer<typeof TimelineSchema>;
