import { z, ZodError } from 'zod';
import { Result } from '@utils/fp';
import { modelToResult } from './utils';

const zDateTime = z.string().transform((value) => Date.parse(value));

export const userModel = z.object({
  id: z.number(),
  name: z.string(),
  email: z.string(),
  fullName: z.optional(z.string()),
  role: z.string(),
  isDisabled: z.boolean(),
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
