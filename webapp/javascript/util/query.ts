import { Maybe } from '@webapp/util/fp';

export function appNameToQuery(appName: string) {
  return `${appName}{}`;
}

export function queryToAppName(query: string): Maybe<string> {
  if (!query || !query.length) {
    return Maybe.nothing();
  }

  const rep = query.replace(/\{.*/g, '');

  if (!rep.length) {
    return Maybe.nothing();
  }

  return Maybe.just(rep);
}
