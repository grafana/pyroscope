import { Result } from '@webapp/util/fp';
import { Target, targetsModel } from '@webapp/models/targets';
import type { ZodError } from 'zod';
import { request, parseResponse } from './base';
import type { RequestError } from './base';

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
