import { Maybe } from '@pyroscope/util/fp';

// ParseQuery parses a string of $app_name<{<$tag_matchers>}> form.
// It assumes the query is well formed
export function parse(query: Query):
  | {
      profileId: string;
      tags?: Record<string, string>;
    }
  | undefined {
  const regex = /(.+){(.*)}/;
  const match = query?.match?.(regex);

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

// Nominal typing
// https://basarat.gitbook.io/typescript/main-1/nominaltyping
enum QueryBrand {
  _ = '',
}
export type Query = QueryBrand & string;

export function brandQuery(query: string) {
  return query as unknown as Query;
}

export function queryFromAppName(appName: string): Query {
  return `${appName}{}` as unknown as Query;
}

export function queryToAppName(q: Query): Maybe<string> {
  const query: string = q;

  if (!query || !query.length) {
    return Maybe.nothing();
  }

  const rep = query.replace(/\{.*/g, '');

  if (!rep.length) {
    return Maybe.nothing();
  }

  return Maybe.just(rep);
}
