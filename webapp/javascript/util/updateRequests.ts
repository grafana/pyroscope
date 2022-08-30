// TODO: figure out the correct types

export function buildRenderURL(
  state: {
    from: string;
    until: string;
    query: string;
    refreshToken?: string;
    maxNodes?: string | number;
    groupBy?: string;
    groupByValue?: string;
  },
  fromOverride?: string,
  untilOverride?: string
) {
  const url = new URL('render', location.href);

  const params = url.searchParams;
  params.set('query', state.query);
  params.set('from', fromOverride || state.from);
  params.set('until', untilOverride || state.until);
  state.refreshToken && params.set('refreshToken', state.refreshToken);
  state.maxNodes && params.set('max-nodes', String(state.maxNodes));
  state.groupBy && params.set('groupBy', state.groupBy);
  state.groupByValue && params.set('groupByValue', state.groupByValue);

  return url.toString();
}

export function buildMergeURLWithQueryID(state: {
  queryID: string;
  refreshToken?: string;
  maxNodes?: string | number;
}) {
  const url = new URL('merge', location.href);

  const params = url.searchParams;
  params.set('queryID', state.queryID);
  state.refreshToken && params.set('refreshToken', state.refreshToken);
  state.maxNodes && params.set('max-nodes', String(state.maxNodes));

  return url.toString();
}

// TODO: merge buildRenderURL and buildDiffRenderURL
export function buildDiffRenderURL(state: {
  from: string;
  until: string;
  leftFrom: string;
  leftUntil: string;
  rightFrom: string;
  rightUntil: string;
  refreshToken?: string;
  maxNodes: string;
  query: string;
}) {
  const { from, until, leftFrom, leftUntil, rightFrom, rightUntil } = state;
  const urlStr = buildRenderURL(state, from, until);
  const url = new URL(urlStr, location.href);

  url.pathname = url.pathname.replace('render', 'render-diff');

  const params = url.searchParams;
  params.set('leftFrom', leftFrom);
  params.set('leftUntil', leftUntil);
  params.set('rightFrom', rightFrom);
  params.set('rightUntil', rightUntil);

  return url.toString();
}
