import { Result } from '@pyroscope/util/fp';
import { Target, targetsModel } from '@pyroscope/models/targets';
import type { ZodError } from 'zod';
import { request, parseResponse } from '@pyroscope/services/base';
import type { RequestError } from '@pyroscope/services/base';

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
