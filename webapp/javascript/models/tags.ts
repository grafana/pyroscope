import { Result } from '@webapp/util/fp';
import { z, ZodError } from 'zod';
import { modelToResult } from './utils';

export const TagsSchema = z.array(z.string()).transform((ar) => {
  // Strip '__name__' since from user perspective it's not really a label
  return ar.filter((a) => a !== '__name__');
});
export type Tags = z.infer<typeof TagsSchema>;

export const TagsValuesSchema = z.array(z.string());
export type TagsValues = z.infer<typeof TagsValuesSchema>;

export function parse(a: unknown): Result<Tags, ZodError> {
  return modelToResult(TagsSchema, a);
}
