import { Result } from '@webapp/util/fp';
import { AppNames, appNamesModel } from '@webapp/models/appNames';
import type { ZodError } from 'zod';
import { parseResponse, request } from './base';
import type { RequestError } from './base';

/* eslint-disable import/prefer-default-export */
export interface FetchAppNamesError {
  message?: string;
}

// Due to circunstances, older versions of pyroscope accepted apps with empty names
// TODO: maybe also check for illegal characters and whatnot?
function isValidAppName(appName: string) {
  return appName.trim().length > 0;
}

export async function fetchAppNames(
  abortController: AbortController
): Promise<Result<AppNames, RequestError | ZodError>> {
  const response = await request('/label-values?label=__name__', {
    signal: abortController.signal,
  });

  if (response.isOk) {
    return parseResponse<AppNames>(response, appNamesModel).map((values) =>
      values.filter(isValidAppName)
    );
  }

  return Result.err<AppNames, RequestError>(response.error);
}
