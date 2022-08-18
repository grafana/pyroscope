import { z, ZodError } from 'zod';
import { Result } from '@webapp/util/fp';
import { modelToResult } from './utils';

export const appModel = z.object({
  name: z.string(),
});

export const appsModel = z.array(appModel);

export type Apps = z.infer<typeof appsModel>;
export type App = z.infer<typeof appModel>;

export function parse(a: unknown): Result<Apps, ZodError> {
  return modelToResult(appsModel, a);
}
