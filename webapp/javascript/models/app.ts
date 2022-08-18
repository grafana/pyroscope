import { z } from 'zod';

export const appModel = z.object({
  name: z.string(),
});

export const appsModel = z.array(appModel);

export type Apps = z.infer<typeof appsModel>;
export type App = z.infer<typeof appModel>;
