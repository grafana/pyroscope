// TODO: figure out the correct types

export function buildRenderURL(
  state: {
    from: string;
    until: string;
    query: string;
    refreshToken?: string;
    maxNodes: string | number;
  },
  fromOverride?: string,
  untilOverride?: string
) {
  let { from, until, query } = state;

  if (fromOverride) {
    from = fromOverride;
  }

  if (untilOverride) {
    until = untilOverride;
  }

  let url = `render?from=${encodeURIComponent(from)}&until=${encodeURIComponent(
    until
  )}`;

  url += `&query=${encodeURIComponent(query)}`;

  if (state.refreshToken) {
    url += `&refreshToken=${state.refreshToken}`;
  }
  url += `&max-nodes=${state.maxNodes}`;

  return url;
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
  let { from, until, leftFrom, leftUntil, rightFrom, rightUntil } = state;
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
