import { Result } from '@utils/fp';
import { APIKeys, parse } from '@models/apikeys';
import type { ZodError } from 'zod';
import { request } from './base';
import type { RequestError } from './base';

export interface FetchAPIKeysError {
  message?: string;
}

export async function fetchAPIKeys(): Promise<
  Result<APIKeys, RequestError | ZodError>
> {
  const response = await request('/api/keys');

  try {
    if (response.isOk) {
      console.log('Parsing', response.value);
      return parse(response.value);
    }
  } catch (e) {
    console.error(e);
  }

  return Result.err<APIKeys, RequestError>(response.error);
}
