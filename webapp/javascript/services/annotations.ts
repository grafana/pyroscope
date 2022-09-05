import { Result } from '@webapp/util/fp';
import type { ZodError } from 'zod';
import { Annotation, AnnotationSchema } from '@webapp/models/annotation';
import { request, parseResponse } from './base';
import type { RequestError } from './base';

export interface NewAnnotation {
  appName: string;
  content: string;
  // TODO(eh-am): number or date?
  timestamp: number;
}
export async function addAnnotation(
  data: NewAnnotation
): Promise<Result<Annotation[], RequestError | ZodError>> {
  const response = await request('/api/annotations', {
    method: 'POST',
    body: JSON.stringify(data),
  });
  return parseResponse<Annotation[]>(response, AnnotationSchema);
}
