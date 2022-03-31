import { Result } from '@webapp/util/fp';
import type { ZodError } from 'zod';
import { modelToResult } from '@webapp/models/utils';
import { request, RequestError } from './base';

type Response = Awaited<ReturnType<typeof request>>;
type Schema = Parameters<typeof modelToResult>[0];

// parseResponse parses a response with given schema if the request has not failed
// eslint-disable-next-line import/prefer-default-export
export function parseResponse<T>(
  res: Response,
  schema: Schema
): Result<T, RequestError | ZodError> {
  if (res.isErr) {
    return Result.err<T, RequestError>(res.error);
  }

  return modelToResult(schema, res.value) as Result<T, ZodError<ShamefulAny>>;
}
