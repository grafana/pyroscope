import {
  Tags,
  TagsValuesSchema,
  TagsValues,
  TagsSchema,
} from '@webapp/models/tags';
import { request, parseResponse } from './base';

export async function fetchTags(query: string) {
  const response = await request(`labels?query=${query}`);
  return parseResponse<Tags>(response, TagsSchema);
}

export async function fetchLabelValues(label: string, query: string) {
  const response = await request(`label-values?label=${label}&query=${query}`);
  return parseResponse<TagsValues>(response, TagsValuesSchema);
}
