/* eslint-disable import/prefer-default-export */
import { Result } from '@utils/fp';
import basename from '../util/baseurl';

interface RequestError {
  statusCode: number;
  message: string;
}

export async function get<T>(
  path: string,
  config?: RequestInit
): Promise<Result<T, RequestError>> {
  let baseURL = basename();

  // There's no explicit baseURL configured
  // So let's try to infer one
  // This is useful for eg in tests
  if (!baseURL) {
    baseURL = window.location.href;
  }

  const address = new URL(path, baseURL).href;

  const response = await fetch(address, config);
  // TODO 1
  // Response could fail (request never returns)
  //
  // TODO 2
  // response can fail with a status code for eg
  if (!response.ok) {
    // Check if there's a body
    // If so, use it

    return Result.err({
      statusCode: response.status,
      message: 'Request failed',
    });
  }

  return Result.ok('hey' as unknown as T);
}
