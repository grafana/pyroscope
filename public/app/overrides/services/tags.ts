import {
  Tags,
  TagsValuesSchema,
  TagsValues,
  TagsSchema,
} from '@webapp/models/tags';
import { parseResponse, request } from '@webapp/services/base';

export async function fetchTags(query: string, from: number, until: number) {
  const response = await request('/pyroscope/labels');
  const isMetaTag = (tag: string) => tag.startsWith('__') && tag.endsWith('__');

  return parseResponse<Tags>(
    response,
    TagsSchema.transform((tags) => {
      return tags.filter((t) => !isMetaTag(t));
    })
  );
}

export async function fetchLabelValues(
  label: string,
  query: string,
  from: number,
  until: number
) {
  const searchParams = new URLSearchParams({
    label,
  });
  const response = await request('/pyroscope/label-values?' + searchParams);

  return parseResponse<TagsValues>(response, TagsValuesSchema);
}
