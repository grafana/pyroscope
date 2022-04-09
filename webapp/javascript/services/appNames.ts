import { Result } from '@webapp/util/fp';
import { AppNames, parse } from '@webapp/models/appNames';
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

// Due to circunstances, older versions of pyroscope accepted apps with empty names
// TODO: maybe also check for illegal characters and whatnot?
function isValidAppName(appName: string) {
  return appName.trim().length > 0;
}

export async function fetchAppNames(
  props?: FetchAppNamesProps
): Promise<Result<AppNames, RequestError | ZodError>> {
  const response = await request('label-values?label=__name__');

  if (response.isOk) {
    return parse(response.value).map((values) => values.filter(isValidAppName));
  }

  return Result.err<AppNames, RequestError>(response.error);
}
