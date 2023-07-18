import { App, appsModel } from '@webapp/models/app';
import { Result } from '@webapp/util/fp';
import type { ZodError } from 'zod';
import type { RequestError } from './base';
import { parseResponse, request } from './base';

export interface FetchAppsError {
  message?: string;
}

export async function fetchApps(): Promise<
  Result<App[], RequestError | ZodError>
> {
  const response = await request('/api/apps');

  if (response.isOk) {
    return parseResponse(response, appsModel);
  }

  return Result.err<App[], RequestError>(response.error);
}

export async function deleteApp(data: {
  name: string;
}): Promise<Result<boolean, RequestError | ZodError>> {
  const { name } = data;
  const response = await request(`/api/apps`, {
    method: 'DELETE',
    body: JSON.stringify({ name }),
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<boolean, RequestError>(response.error);
}
