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
