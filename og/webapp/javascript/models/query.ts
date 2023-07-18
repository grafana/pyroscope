import { Maybe } from '@webapp/util/fp';

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
