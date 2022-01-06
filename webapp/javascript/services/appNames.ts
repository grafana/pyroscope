import { Result } from '@utils/fp';
import { AppNames } from '@models/appNames';
import { request } from './base';
// import fetch from 'cross-fetch';

/* eslint-disable import/prefer-default-export */
interface FetchAppNamesProps {
  abortCtrl?: AbortController;
}

export interface FetchAppNamesError {
  message?: string;
}

export async function fetchAppNames(
  props?: FetchAppNamesProps
): Promise<Result<AppNames, FetchAppNamesError>> {
  const response = await request('label-values?label=__name__');

  return response;
  //  response.map();
  //  console.log({ response });
  //  if (!response.ok) {
  //    return Result.err({
  //      message: `Response not ok. Status code ${response.status}`,
  //    });
  //  }
  //  //  const data = await response.json();
  //
  //  return Result.ok([]);
}
