export function buildRenderURL(state, fromOverride=null, untilOverride=null, side=null) {
  let { from, until } = state;

  if (fromOverride) {
    from = fromOverride;
  }

  if (untilOverride) {
    until = untilOverride;
  }

  let url = `render?from=${encodeURIComponent(from)}&until=${encodeURIComponent(until)}`;
  const nameLabel = state.labels.find((x) => x.name == '__name__');

  if (nameLabel) {
    url += `&name=${nameLabel.value}{`;
  } else {
    url += '&name=unknown{';
  }

  // TODO: replace this so this is a real utility function
  url += state.labels.filter((x) => x.name != '__name__').map((x) => `${x.name}=${x.value}`).join(',');
  url += '}';

  if (state.refreshToken) {
    url += `&refreshToken=${state.refreshToken}`;
  }
  url += `&max-nodes=${state.maxNodes}`;

  return url;
}

// TODO: merge buildRenderURL and buildDiffRenderURL
export function buildDiffRenderURL(state, { from: fromOverride, until: untilOverride, leftFrom: leftFromOverride, leftUntil: leftUntilOverride, rightFrom: rightFromOverride, rightUntil: rightUntilOverride } = {}) {
  let { from, until, leftFrom, leftUntil, rightFrom, rightUntil } = state;
  from = fromOverride || from;
  until = untilOverride || until;
  leftFrom = leftFromOverride || leftFrom;
  leftUntil = leftUntilOverride || leftUntil;
  rightFrom = rightFromOverride || rightFrom;
  rightUntil = rightUntilOverride || rightUntil;

  const urlStr = buildRenderURL(state, from, until);
  const url = new URL(urlStr, location.origin);
  url.pathname = '/render-diff'; // TODO: merge with buildRenderURL

  const params = url.searchParams;
  params.set('leftFrom', leftFrom);
  params.set('leftUntil', leftUntil);
  params.set('rightFrom', rightFrom);
  params.set('rightUntil', rightUntil);

  return url.toString();
}
