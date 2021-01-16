// TODO: copy logic from go
export function parseLabels(v) {
  const res = [];
  if (v) {
    let [a, b] = v.split('{');
    b = b.split('}')[0];
    res.push({ name: '__name__', value: a });
    b.split(',').forEach((x) => {
      if (x) {
        const [k, v] = x.split('=');
        res.push({ name: k, value: v });
      }
    });
  }
  return res;
}

export function encodeLabels(v) {
  let res = '';
  const nameLabel = v.find((x) => x.name == '__name__');
  if (nameLabel) {
    res += `${nameLabel.value}{`;
  } else {
    res += 'unknown{';
  }
  res += v.filter((x) => x.name !== '__name__').map((x) => `${x.name}=${x.value}`).join(',');
  res += '}';
  return res;
}
