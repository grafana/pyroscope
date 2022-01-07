import { Result } from '@utils/fp';
import { AppNames, parse } from '@models/appNames';
import type { ZodError } from 'zod';
import { request } from './base';
import type { RequestError } from './base';

/* eslint-disable import/prefer-default-export */
interface FetchAppNamesProps {
  abortCtrl?: AbortController;
}

export interface FetchAppNamesError {
  message?: string;
}

export async function fetchAppNames(
  props?: FetchAppNamesProps
): Promise<Result<AppNames, RequestError | ZodError>> {
  const response = await request('label-values?label=__name__');

  if (response.isOk) {
    return parse(response.value);
  }

  return Result.err<AppNames, RequestError>(response.error);
}
