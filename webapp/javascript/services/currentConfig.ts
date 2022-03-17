import { Result } from '@utils/fp';
import { CurrentConfig, parse } from '@models/currentConfig';
import { ZodError } from 'zod';
import { request } from './base';
import type { RequestError } from './base';

/* eslint-disable import/prefer-default-export */
export async function fetchCurrentConfig(): Promise<
  Result<CurrentConfig, RequestError | ZodError>
> {
  const response = await request('status/config');

  if (response.isOk) {
    return parse(response.value);
  }

  return Result.err<CurrentConfig, RequestError>(response.error);
}
