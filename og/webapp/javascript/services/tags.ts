import {
  Tags,
  TagsValuesSchema,
  TagsValues,
  TagsSchema,
} from '@webapp/models/tags';
import { request, parseResponse } from './base';

export async function fetchTags(query: string, from: number, until: number) {
  const params = new URLSearchParams({
    query,
    from: from.toString(10),
    until: until.toString(10),
  });
  const response = await request(`/labels?${params.toString()}`);
  return parseResponse<Tags>(response, TagsSchema);
}

export async function fetchLabelValues(
  label: string,
  query: string,
  from: number,
  until: number
) {
  const params = new URLSearchParams({
    query,
    label,
    from: from.toString(10),
    until: until.toString(10),
  });
  const response = await request(`/label-values?${params.toString()}`);
  return parseResponse<TagsValues>(response, TagsValuesSchema);
}
