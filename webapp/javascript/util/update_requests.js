export function buildRenderURL(state) {
  const { from, until } = state;

  let url = `/render?from=${encodeURIComponent(from)}&until=${encodeURIComponent(until)}`;
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
