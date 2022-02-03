import { Result } from '@utils/fp';
import { APIKey, APIKeys, parse, apikeyModel } from '@models/apikeys';
import { modelToResult } from '@models/utils';
import { request } from './base';
import type { RequestError } from './base';

export interface FetchAPIKeysError {
  message?: string;
}

export async function fetchAPIKeys(): Promise<
  Result<APIKeys, RequestError | ZodError>
> {
  const response = await request('/api/keys');
  if (response.isOk) {
    return parse(response.value);
  }

  return Result.err<APIKeys, RequestError>(response.error);
}

export async function createAPIKey(
  data
): Promise<Result<APIKeys, RequestError | ZodError>> {
  const response = await request('/api/keys', {
    method: 'POST',
    body: JSON.stringify(data),
  });

  if (response.isOk) {
    return modelToResult<APIKey>(apikeyModel, response.value);
  }

  return Result.err<APIKeys, RequestError>(response.error);
}

export async function deleteAPIKey(
  data
): Promise<Result<APIKeys, RequestError | ZodError>> {
  console.log(data);
  const response = await request(`/api/${data.id}`, {
    method: 'DELETE',
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<APIKeys, RequestError>(response.error);
}
