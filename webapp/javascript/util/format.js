import murmurhash3_32_gc from './murmur3';

export function numberWithCommas(x) {
  return x.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

export function colorBasedOnPackageName(name, a) {
  const purple = `hsla(246, 40%, 65%, ${a})` //Purple:
  const blueDark = `hsla(211, 48%, 60%, ${a})` //BlueDark:
  const blueCyan = `hsla(194, 52%, 61%, ${a})` //CyanBlue:
  const yellow = `hsla(34, 65%, 65%, ${a})` //Yellow:
  const green = `hsla(163, 45%, 55%, ${a})` //Green:
  const orange = `hsla(24, 69%, 60%, ${a})` //Orange:
  const red = `hsla(3, 62%, 67%, ${a})` // Red:
  const grey = `hsla(225, 2%, 51%, ${a})` //Grey:

  const items = [
    // red,
    orange,
    yellow,
    green,
    blueCyan,
    blueDark,
    purple,
  ]

  // const darkGreen = `hsla(160, 40%, 21%, ${a})` //Dark green:
  // const darkPurple = `hsla(240, 30%, 29%, ${a})` //puprple:
  // const darkBlue = `hsla(226, 36%, 26%, ${a})` //Dark blue:
  // const darkPink = `hsla(315, 40%, 24%, ${a})` //Pink:
  // const darkYellow = `hsla(62, 29%, 22%, ${a})` //Yellow/mustard:
  // const darkRed = `hsla(10, 41%, 23%, ${a})` //Red:
  //
  // const items = [
  //   darkGreen,
  //   darkPurple,
  //   darkBlue,
  //   darkPink,
  //   darkYellow,
  //   darkRed,
  // ]

  let colorIndex = murmurhash3_32_gc(name) % items.length;
  return items[colorIndex];
}

export function colorGreyscale(v, a) {
  return `rgba(${v}, ${v}, ${v}, ${a})`;
}
