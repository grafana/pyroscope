import { parseResponse, requestWithOrgID } from '@webapp/services/base';
import { z } from 'zod';

const seriesLabelsSchema = z.preprocess(
  (a: any) => {
    if ('labelsSet' in a) {
      return a;
    }

    return { labelsSet: [{ labels: [] }] };
  },
  z.object({
    labelsSet: z.array(
      z.object({
        labels: z.array(
          z.object({
            name: z.string(),
            value: z.string(),
          })
        ),
      })
    ),
  })
);

async function fetchLabelsSeries<T>(
  query: string,
  transformFn: (t: Array<{ name: string; value: string }>) => T
) {
  const profileTypeID = query.replace(/\{.*/g, '');
  const response = await requestWithOrgID('/querier.v1.QuerierService/Series', {
    method: 'POST',
    body: JSON.stringify({
      matchers: [`{__profile_type__=\"${profileTypeID}\"}`],
    }),
    headers: {
      'content-type': 'application/json',
    },
  });
  const isMetaTag = (tag: string) => tag.startsWith('__') && tag.endsWith('__');

  return parseResponse<T>(
    response,
    seriesLabelsSchema
      .transform((res) => {
        return res.labelsSet
          .flatMap((a) => a.labels)
          .filter((a) => !isMetaTag(a.name));
      })
      .transform(transformFn)
  );
}

export async function fetchTags(query: string, from: number, until: number) {
  return fetchLabelsSeries(query, function (t) {
    const labelNames = t.map((a) => a.name);
    return Array.from(new Set(labelNames));
  });
}

export async function fetchLabelValues(
  label: string,
  query: string,
  from: number,
  until: number
) {
  return fetchLabelsSeries(query, function (t) {
    const labelValues = t.filter((l) => label === l.name).map((a) => a.value);
    return Array.from(new Set(labelValues));
  });
}
