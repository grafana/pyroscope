import { Result } from '@pyroscope/util/fp';
import type { ZodError } from 'zod';
import { Annotation, AnnotationSchema } from '@pyroscope/models/annotation';
import { request, parseResponse } from './base';
import type { RequestError } from './base';

export interface NewAnnotation {
  appName: string;
  content: string;
  timestamp: number;
}
export async function addAnnotation(
  data: NewAnnotation
): Promise<Result<Annotation, RequestError | ZodError>> {
  const response = await request('/api/annotations', {
    method: 'POST',
    body: JSON.stringify(data),
  });
  return parseResponse(response, AnnotationSchema);
}
