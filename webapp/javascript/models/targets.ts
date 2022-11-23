import { z } from 'zod';

const healthModel = z.enum(['up', 'down', 'unknown']);
const targetModel = z.object({
  discoveredLabels: z.record(z.string()),
  labels: z.record(z.string()),
  job: z.string(),
  url: z.string(),
  lastError: z.optional(z.string()),
  lastScrape: z.string(),
  lastScrapeDuration: z.string(),
  health: healthModel,
});
export const targetsModel = z.array(targetModel);

export type Target = z.infer<typeof targetModel>;
export type Health = z.infer<typeof healthModel>;
