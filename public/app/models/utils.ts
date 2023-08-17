import type { ZodError, ZodType } from 'zod';
import { Result } from '@pyroscope/util/fp';

/**
 * modelToResult converts a (most likely) zod model into a Result
 */
// eslint-disable-next-line import/prefer-default-export
export function modelToResult<T>(
  s: ZodType<T>,
  data: unknown
): Result<T, ZodError> {
  const result = s.safeParse(data);

  // TODO check why this is failing
  if (!result.success) {
    return Result.err((result as ShamefulAny).error);
  }

  return Result.ok(result.data);
}
