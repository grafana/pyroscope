import { z, ZodError } from 'zod';
import { Result } from '@utils/fp';
import { modelToResult } from './utils';

const zDateTime = z.string().transform((value) => {
  if (typeof value === 'string') {
    return Date.parse(value);
  }
  if (typeof value === 'number') {
    return new Date(value);
  }
  return value;
});

export const userModel = z.object({
  id: z.number(),
  name: z.string(),
  email: z.optional(z.string()),
  fullName: z.optional(z.string()),
  role: z.string(),
  isDisabled: z.boolean(),
  isExternal: z.optional(z.boolean()),
  createdAt: zDateTime,
  updatedAt: zDateTime,
  passwordChangedAt: zDateTime,
});

export const usersModel = z.array(userModel);

export type Users = z.infer<typeof usersModel>;
export type User = z.infer<typeof userModel>;

export function parse(a: unknown): Result<Users, ZodError> {
  return modelToResult(usersModel, a);
}

export const passwordEncode = (p) => btoa(unescape(encodeURIComponent(p)));
