import {
  App,
  AppSchema,
  PyroscopeAppLabel,
  ServiceNameLabel,
} from '@webapp/models/app';
import { Result } from '@webapp/util/fp';
import { z, ZodError } from 'zod';
import type { RequestError } from '@webapp/services/base';
import { parseResponse, requestWithOrgID } from '@webapp/services/base';

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

export async function fetchApps(): Promise<
  Result<App[], RequestError | ZodError>
> {
  // TODO: is this the best query?
  const response = await requestWithOrgID('/querier.v1.QuerierService/Series', {
    method: 'POST',
    body: JSON.stringify({
      matchers: [],
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
