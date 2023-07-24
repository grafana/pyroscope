import { Result } from '@phlare/util/fp';
import { Target, targetsModel } from '@phlare/models/targets';
import type { ZodError } from 'zod';
import { request, parseResponse } from '@phlare/services/base';
import type { RequestError } from '@phlare/services/base';

/* eslint-disable import/prefer-default-export */
export async function fetchTargets(): Promise<
  Result<Target[], RequestError | ZodError>
> {
  const response = await request('targets');

  if (response.isOk) {
    return parseResponse(response, targetsModel);
  }

  return Result.err<Target[], RequestError>(response.error);
}
