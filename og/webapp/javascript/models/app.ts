import { SpyNameSchema } from '@pyroscope/models/src';
import { UnitsSchema } from '@pyroscope/models/src/units';
import { z } from 'zod';

export const appModel = z.object({
  name: z.string(),
  spyName: SpyNameSchema,
  units: UnitsSchema,
});

export const appsModel = z.array(appModel);

export type App = z.infer<typeof appModel>;
