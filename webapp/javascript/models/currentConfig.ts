import { Result } from '@utils/fp';
import { z, ZodError } from 'zod';
import { modelToResult } from './utils';

const configModel = z.object({
  yaml: z.string(),
});

export type CurrentConfig = z.infer<typeof configModel>;

export function parse(a: unknown): Result<CurrentConfig, ZodError> {
  return modelToResult(configModel, a);
}
