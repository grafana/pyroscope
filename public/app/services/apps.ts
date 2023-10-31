import {
  App,
  AppSchema,
  appsModel,
  PyroscopeAppLabel,
  ServiceNameLabel,
} from '@pyroscope/models/app';
import { Result } from '@pyroscope/util/fp';
import { z, ZodError } from 'zod';
import type { RequestError } from '@pyroscope/services/base';
import { parseResponse, request } from '@pyroscope/services/base';

// SeriesResponse refers to the response from the server, without any manipulation
const SeriesResponseSchema = z.preprocess(
  (arg) => {
    const noop = { labelsSet: [] };
    if (!arg || typeof arg !== 'object') {
      return noop;
    }

    // The backend may return an empty object ({})
    if (!('labelsSet' in arg)) {
      return noop;
    }

    return arg;
  },
  z.object({
    labelsSet: z.array(
      z.object({
        labels: z.array(z.object({ name: z.string(), value: z.string() })),
      })
    ),
  })
);
type SeriesResponse = z.infer<typeof SeriesResponseSchema>;

// Transform SeriesResponseSchema in a list of applications
// It:
// * flattens all labels from the same labelSet into an object (App)
// * remove duplicates
const ListOfAppsSchema = SeriesResponseSchema.transform(flattenAndMergeLabels)
  .transform(removeWithoutRequiredLabels)
  .pipe(z.array(AppSchema))
  .transform(removeDuplicateApps);

function removeWithoutRequiredLabels(
  s: ReturnType<typeof flattenAndMergeLabels>
) {
  return s.filter((a) => {
    return PyroscopeAppLabel in a || ServiceNameLabel in a;
  });
}

function flattenAndMergeLabels(s: SeriesResponse) {
  return s.labelsSet.map((v) => {
    return v.labels.reduce((acc, curr) => {
      acc[curr.name] = curr.value;
      return acc;
    }, {} as Record<string, string>);
  });
}

function removeDuplicateApps(app: App[]) {
  const idFn = (b: (typeof app)[number]) => `${b.__profile_type__}-${b.name}`;

  const visited = new Set<string>();

  return app.filter((b) => {
    // TODO: it may be possible that the same "app" belongs to different languages
    // with this code we only use the first one
    if (visited.has(idFn(b))) {
      return false;
    }

    visited.add(idFn(b));
    return true;
  });
}

export async function fetchApps(
  fromMs?: number,
  untilMs?: number
): Promise<Result<App[], RequestError | ZodError>> {
  let response = await request('/querier.v1.QuerierService/Series', {
    method: 'POST',
    body: JSON.stringify({
      matchers: [],
      labelNames: [
        PyroscopeAppLabel,
        ServiceNameLabel,
        '__profile_type__',
        '__type__',
        '__name__',
      ],
      start: fromMs || 0,
      end: untilMs || 0,
    }),
    headers: {
      'content-type': 'application/json',
    },
  });

  if (response.isOk) {
    return parseResponse(response, ListOfAppsSchema);
  }

  // try without labelNames in case of an error since this has been added in a later version
  response = await request('/querier.v1.QuerierService/Series', {
    method: 'POST',
    body: JSON.stringify({
      matchers: [],
      start: fromMs,
      end: untilMs,
    }),
    headers: {
      'content-type': 'application/json',
    },
  });
  if (response.isOk) {
    return parseResponse(response, ListOfAppsSchema);
  }

  return Result.err<App[], RequestError>(response.error);
}

export interface FetchAppsError {
  message?: string;
}

export async function fetchAppsOG(): Promise<
  Result<App[], RequestError | ZodError>
> {
  const response = await request('/api/apps');

  if (response.isOk) {
    return parseResponse(response, appsModel);
  }

  return Result.err<App[], RequestError>(response.error);
}

export async function deleteApp(data: {
  name: string;
}): Promise<Result<boolean, RequestError | ZodError>> {
  const { name } = data;
  const response = await request(`/api/apps`, {
    method: 'DELETE',
    body: JSON.stringify({ name }),
  });

  if (response.isOk) {
    return Result.ok(true);
  }

  return Result.err<boolean, RequestError>(response.error);
}
