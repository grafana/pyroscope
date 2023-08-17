import { z } from 'zod';

const GroupSchema = z.object({
  watermark: z.object({}).optional(),
  // timeline data
  startTime: z.number(),
  samples: z.array(z.number()),
  durationDelta: z.number(),
});

export const GroupsSchema = z.record(z.string(), GroupSchema);

export type Groups = z.infer<typeof GroupsSchema>;
export type Group = z.infer<typeof GroupSchema>;
