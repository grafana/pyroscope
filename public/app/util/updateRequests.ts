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
    aggregation?: string;
  },
  fromOverride?: string,
  untilOverride?: string
) {
  const params = new URLSearchParams();
  params.set('query', state.query);
  params.set('from', fromOverride || state.from);
  params.set('until', untilOverride || state.until);
  state.refreshToken && params.set('refreshToken', state.refreshToken);
  if (state.maxNodes && state.maxNodes !== '0') {
    params.set('max-nodes', String(state.maxNodes));
  }
  state.groupBy && params.set('groupBy', state.groupBy);
  state.groupByValue && params.set('groupByValue', state.groupByValue);
  state.aggregation && params.set('aggregation', state.aggregation);

  return `/render?${params}`;
}

export function buildMergeURLWithQueryID(state: {
  queryID: string;
  refreshToken?: string;
  maxNodes?: string | number;
  aggregation?: string;
}) {
  const params = new URLSearchParams();
  params.set('queryID', state.queryID);
  state.refreshToken && params.set('refreshToken', state.refreshToken);
  if (state.maxNodes && state.maxNodes !== '0') {
    params.set('max-nodes', String(state.maxNodes));
  }
  state.aggregation && params.set('aggregation', state.aggregation);

  return `/merge?${params}`;
}
