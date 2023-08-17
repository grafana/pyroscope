import { Result } from '@pyroscope/util/fp';
import type { ZodError } from 'zod';
import {
  APIKeys,
  apikeyModel,
  apiKeysSchema,
  APIKey,
} from '@pyroscope/models/apikeys';
import { request, parseResponse } from './base';
import type { RequestError } from './base';

export interface FetchAPIKeysError {
  message?: string;
}

export async function fetchAPIKeys(): Promise<
  Result<APIKeys, RequestError | ZodError>
> {
  const response = await request('/api/keys');
  return parseResponse<APIKeys>(response, apiKeysSchema);
}

export async function createAPIKey(data: {
  name: string;
  role: string;
  ttlSeconds: number;
}): Promise<Result<APIKey, RequestError | ZodError>> {
  const response = await request('/api/keys', {
    method: 'POST',
    body: JSON.stringify(data),
  });

  return parseResponse(response, apikeyModel);
}

export async function deleteAPIKey(data: {
  id: number;
}): Promise<Result<unknown, RequestError | ZodError>> {
  const response = await request(`/api/keys/${data.id}`, {
    method: 'DELETE',
  });

  if (response.isOk) {
    return Result.ok();
  }

  return Result.err<APIKeys, RequestError>(response.error);
}
