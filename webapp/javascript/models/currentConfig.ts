import { z } from 'zod';

export const CurrentConfigSchema = z.object({
  yaml: z.string(),
});

export type CurrentConfig = z.infer<typeof CurrentConfigSchema>;
