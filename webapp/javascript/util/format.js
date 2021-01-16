import murmurhash3_32_gc from './murmur3';

export function numberWithCommas(x) {
  return x.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export function colorBasedOnPackageName(name, a) {
  const purple = `hsla(246, 40%, 65%, ${a})`; // Purple
  const blueDark = `hsla(211, 48%, 60%, ${a})`; // BlueDark
  const blueCyan = `hsla(194, 52%, 61%, ${a})`; // CyanBlue
  const yellowDark = `hsla(34, 65%, 65%, ${a})`; // Dark Yellow
  const yellowLight = `hsla(47, 100%, 73%, ${a})`; // Light Yellow
  const green = `hsla(163, 45%, 55%, ${a})`; // Green
  const orange = `hsla(24, 69%, 60%, ${a})`; // Orange
  const pink = `hsla(305, 63%, 79%, ${a})`; // Pink
  // const red = `hsla(3, 62%, 67%, ${a})` // Red
  // const grey = `hsla(225, 2%, 51%, ${a})` //Grey

  const items = [
    // red,
    orange,
    yellowDark,
    blueCyan,
    green,
    blueDark,
    purple,
    pink,
    yellowLight,
  ];

  const colorIndex = murmurhash3_32_gc(name) % items.length;
  return items[colorIndex];
}

export function colorGreyscale(v, a) {
  return `rgba(${v}, ${v}, ${v}, ${a})`;
}
