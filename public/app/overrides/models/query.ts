import { Query } from '@pyroscope/webapp/javascript/models/query';

export * from '@pyroscope/webapp/javascript/models/query';

// ParseQuery parses a string of $app_name<{<$tag_matchers>}> form.
// It assumes the query is well formed
export function parse(query: Query):
  | {
      profileId: string;
      tags?: Record<string, string>;
    }
  | undefined {
  const regex = /(.+){(.*)}/;
  const match = query.match(regex);

  if (!match) {
    // TODO: return a Nothing() ?
    return undefined;
  }

  const [_original, head, tail] = match;
  const tags = parseTags(tail);

  if (!Object.keys(tags).length) {
    return { profileId: head };
  }
  return { profileId: head, tags };
}

function parseTags(s: string) {
  const pattern = /(\w+)="([^"]+)/g;

  let match;
  const matches: Record<string, string> = {};

  while ((match = pattern.exec(s)) !== null) {
    const key = match[1];
    const value = match[2];
    matches[key] = value;
  }

  return matches;
}
