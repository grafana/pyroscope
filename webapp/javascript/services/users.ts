import { Result } from '@utils/fp';
import { Users, parse } from '@models/users';
import type { ZodError } from 'zod';
import { request } from './base';
import type { RequestError } from './base';

export interface FetchUsersError {
  message?: string;
}

export async function fetchUsers(): Promise<
  Result<Users, RequestError | ZodError>
> {
  const response = await request('/api/users');

  try {
    if (response.isOk) {
      console.log('Parsing', response.value);
      return parse(response.value);
    }
  } catch (e) {
    console.error(e);
  }

  return Result.err<Users, RequestError>(response.error);
}
