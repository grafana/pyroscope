import { Result } from '@webapp/util/fp';

export async function fetchTags(query: string, from: number, until: number) {
  return Result.ok<string[], { message: string }>([]);
}

export async function fetchLabelValues(
  label: string,
  query: string,
  from: number,
  until: number
) {
  return Result.err<string[], { message: string }>({
    message: 'TODO: implement ',
  });
}
