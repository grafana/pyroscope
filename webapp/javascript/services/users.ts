import { Result } from '@utils/fp';
import {
  Users,
  parse,
  type User,
  userModel,
  passwordEncode,
} from '@models/users';
import type { ZodError } from 'zod';
import { modelToResult } from '@models/utils';
import { request } from './base';
import type { RequestError } from './base';

export interface FetchUsersError {
  message?: string;
}

export async function fetchUsers(): Promise<
  Result<Users, RequestError | ZodError>
> {
  const response = await request('/api/users');

  if (response.isOk) {
    return parse(response.value);
  }

  return Result.err<Users, RequestError>(response.error);
}

export async function disableUser(
  user: User
): Promise<Result<boolean, RequestError | ZodError>> {
  const response = await request(`/api/users/${user.id}/disable`, {
    method: 'PUT',
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function enableUser(
  user: User
): Promise<Result<boolean, RequestError | ZodError>> {
  const response = await request(`/api/users/${user.id}/enable`, {
    method: 'PUT',
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function createUser(
  user: User
): Promise<Result<boolean, RequestError | ZodError>> {
  const response = await request(`/api/users`, {
    method: 'POST',
    body: JSON.stringify(user),
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function loadCurrentUser(): Promise<
  Result<User | boolean, RequestError | ZodError>
> {
  const response = await request(`/api/user`);
  if (response.isOk) {
    return modelToResult<User>(userModel, response.value);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function changeMyPassword(
  oldPassword: string,
  newPassword: string
): Promise<Result<User, RequestError | ZodError>> {
  const response = await request(`/api/user/password`, {
    method: 'PUT',
    body: JSON.stringify({
      oldPassword: passwordEncode(oldPassword),
      newPassword: passwordEncode(newPassword),
    }),
  });
  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function changeUserRole(
  user: User,
  role: string
): Promise<Result<boolean, RequestError | ZodError>> {
  const response = await request(`/api/users/${user.id}/role`, {
    method: 'PUT',
    body: JSON.stringify({ role }),
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function editMyUser(
  data
): Promise<Result<boolean, RequestError | ZodError>> {
  const response = await request(`/api/users`, {
    method: 'PATCH',
    body: JSON.stringify(data),
  });

  if (response.isOk) {
    return Result.ok(response.value);
  }

  return Result.err<false, RequestError>(response.error);
}

export async function deleteUser(data: {
  id: number;
}): Promise<Result<boolean, RequestError | ZodError>> {
  const { id } = data;
  const response = await request(`/api/users/${id}`, {
    method: 'DELETE',
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<false, RequestError>(response.error);
}
