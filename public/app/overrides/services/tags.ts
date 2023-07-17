import { parseResponse, requestWithOrgID } from '@webapp/services/base';
import { z } from 'zod';

const labelNamesSchema = z.preprocess(
  (a: any) => {
    if ('names' in a) {
      return a;
    }
    return { names: [] };
  },
  z.object({
    names: z.array(z.string()),
  })
);

// Turns a Pyroscope query "goroutine:goroutine:count:goroutine:count{service_name="cortex-dev-01/ruler"}"
// into a list of labels matchers
export function queryToMatchers(query: string) {
  const labelsIndex = query.indexOf('{');
  if (labelsIndex > 0) {
    const profileTypeID = query.substring(0, labelsIndex);
    return [
      `{__profile_type__=\"${profileTypeID}\", ` +
        query.substring(labelsIndex + 1, query.length),
    ];
  }
  if (labelsIndex === 0) {
    return [query];
  }
  return [`{__profile_type__=\"${query}\"}`];
}

export async function fetchTags(query: string, _from: number, _until: number) {
  const response = await requestWithOrgID(
    '/querier.v1.QuerierService/LabelNames',
    {
      method: 'POST',
      body: JSON.stringify({
        matchers: queryToMatchers(query),
      }),
      headers: {
        'content-type': 'application/json',
      },
    }
  );
  const isMetaTag = (tag: string) => tag.startsWith('__') && tag.endsWith('__');

  return parseResponse<string[]>(
    response,
    labelNamesSchema.transform((res) => {
      return Array.from(new Set(res.names.filter((a) => !isMetaTag(a))));
    })
  );
}

export async function fetchLabelValues(
  label: string,
  query: string,
  _from: number,
  _until: number
) {
  const response = await requestWithOrgID(
    '/querier.v1.QuerierService/LabelValues',
    {
      method: 'POST',
      body: JSON.stringify({
        matchers: queryToMatchers(query),
        name: label,
      }),
      headers: {
        'content-type': 'application/json',
      },
    }
  );

  return parseResponse<string[]>(
    response,
    labelNamesSchema.transform((res) => {
      return Array.from(new Set(res.names));
    })
  );
}
