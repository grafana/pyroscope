import { z, ZodError } from 'zod';
import { Result } from '@pyroscope/util/fp';
import { modelToResult } from './utils';

export const flamegraphDotComResponseScheme = z.object({
  url: z.string(),
});

export type FlamegraphDotComResponse = z.infer<
  typeof flamegraphDotComResponseScheme
>;

export function parse(a: unknown): Result<FlamegraphDotComResponse, ZodError> {
  return modelToResult(flamegraphDotComResponseScheme, a);
}
