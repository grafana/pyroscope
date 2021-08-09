export function buildRenderURL(state, fromOverride=null, untilOverride=null, side=null) {
  let { from, until, query } = state;

  if (fromOverride) {
    from = fromOverride;
  }

  if (untilOverride) {
    until = untilOverride;
  }

  let url = `render?from=${encodeURIComponent(from)}&until=${encodeURIComponent(until)}`;

  url += `&query=${encodeURIComponent(query)}`;

  if (state.refreshToken) {
    url += `&refreshToken=${state.refreshToken}`;
  }
  url += `&max-nodes=${state.maxNodes}`;

  return url;
}
