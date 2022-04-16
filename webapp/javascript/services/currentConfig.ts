import { Result } from '@webapp/util/fp';
import {
  CurrentConfig,
  CurrentConfigSchema,
} from '@webapp/models/currentConfig';
import { ZodError } from 'zod';
import { request, parseResponse, RequestError } from './base';

/* eslint-disable import/prefer-default-export */
export async function fetchCurrentConfig(): Promise<
  Result<CurrentConfig, RequestError | ZodError>
> {
  const response = await request('/status/config');
  return parseResponse(response, CurrentConfigSchema);
}
