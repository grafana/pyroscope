import { Result } from '@utils/fp';
import { z, ZodError } from 'zod';
import { modelToResult } from './utils';

export const TagsSchema = z.record(z.string());
export type Tags = z.infer<typeof TagsSchema>;

export const TagsValuesSchema = z.array(z.string());
export type TagsValues = z.infer<typeof TagsValuesSchema>;

export function parse(a: unknown): Result<Tags, ZodError> {
  return modelToResult(TagsSchema, a);
}
