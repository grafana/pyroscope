import { App } from '@webapp/models/app';
import { Result } from '@webapp/util/fp';
import { z } from 'zod';
import type { ZodError } from 'zod';
import type { RequestError } from '@webapp/services/base';
import { parseResponse, request } from '@webapp/services/base';

const appNamesResponse = z.preprocess(
  (arg) => {
    if (!Array.isArray(arg)) {
      return [];
    }

    return arg;
  },
  z.array(z.string()).transform((names) => {
    return names.map((name) => {
      return {
        name,
        spyName: 'gospy',
        units: 'unknown',
      };
    });
  })
);

export async function fetchApps(): Promise<
  Result<App[], RequestError | ZodError>
> {
  const response = await request('/pyroscope/label-values?label=__name__');

  if (response.isOk) {
    return parseResponse(response, appNamesResponse);
  }

  return Result.err<App[], RequestError>(response.error);
}
