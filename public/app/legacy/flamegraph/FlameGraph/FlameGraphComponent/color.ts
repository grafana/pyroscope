/* eslint-disable camelcase */
import Color from 'color';
import { scaleLinear } from 'd3-scale';
import type { SpyName } from '@pyroscope/legacy/models';
import murmurhashThree32GC from './murmur3';
import type { FlamegraphPalette } from './colorPalette';

export const defaultColor = Color.rgb(148, 142, 142);
export const diffColorRed = Color.rgb(200, 0, 0);
export const diffColorGreen = Color.rgb(0, 170, 0);

export const highlightColor = Color('#48CE73');

export function colorBasedOnDiffPercent(
  palette: FlamegraphPalette,
  leftPercent: number,
  rightPercent: number
) {
  const result = diffPercent(leftPercent, rightPercent);
  const color = NewDiffColor(palette);
  return color(result);
}

// TODO move to a different file
// difference between 2 percents
export function diffPercent(leftPercent: number, rightPercent: number) {
  if (leftPercent === rightPercent) {
    return 0;
  }

  if (leftPercent === 0) {
    return 100;
  }

  // https://en.wikipedia.org/wiki/Relative_change_and_difference
  const result = ((rightPercent - leftPercent) / leftPercent) * 100;

  if (result > 100) {
    return 100;
  }
  if (result < -100) {
    return -100;
  }

  return result;
}

export function colorFromPercentage(p: number, alpha: number) {
  // calculated by drawing a line (https://en.wikipedia.org/wiki/Line_drawing_algorithm)
  // where p1 = (0, 180) and p2 = (100, 0)
  // where x is the absolute percentage
  // and y is the color variation
  let v = 180 - 1.8 * Math.abs(p);

  if (v > 200) {
    v = 200;
  }

  // red
  if (p > 0) {
    return Color.rgb(200, v, v).alpha(alpha);
  }
  // green
  if (p < 0) {
    return Color.rgb(v, 200, v).alpha(alpha);
  }
  // grey
  return Color.rgb(200, 200, 200).alpha(alpha);
}

export function colorGreyscale(v: number, a: number) {
  return Color.rgb(v, v, v).alpha(a);
}

function spyToRegex(spyName: SpyName): RegExp {
  // eslint-disable-next-line default-case
  switch (spyName) {
    case 'dotnetspy':
      return /^(?<packageName>.+)\.(.+)\.(.+)\(.*\)$/;
    // TODO: come up with a clever heuristic
    case 'ebpfspy':
      return /^(?<packageName>.+)$/;
    // tested with pyroscope stacktraces here: https://regex101.com/r/99KReq/1
    case 'gospy':
      return /^(?<packageName>.*?\/.*?\.|.*?\.|.+)(?<functionName>.*)$/;
    // assume scrape is golang, since that's the only language we support right now
    case 'scrape':
      return /^(?<packageName>.*?\/.*?\.|.*?\.|.+)(?<functionName>.*)$/;
    case 'phpspy':
      return /^(?<packageName>(.*\/)*)(?<filename>.*\.php+)(?<line_info>.*)$/;
    case 'pyspy':
      return /^(?<packageName>(.*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/;
    case 'rbspy':
      return /^(?<packageName>(.*\/)*)(?<filename>.*\.rb+)(?<line_info>.*)$/;
    case 'nodespy':
      return /^(\.\/node_modules\/)?(?<packageName>[^/]*)(?<filename>.*\.?(jsx?|tsx?)?):(?<functionName>.*):(?<line_info>.*)$/;
    case 'tracing':
      return /^(?<packageName>.+?):.*$/;
    case 'javaspy':
      // TODO: we might want to add ? after groups
      return /^(?<packageName>.+\/)(?<filename>.+\.)(?<functionName>.+)$/;
    case 'pyroscope-rs':
      return /^(?<packageName>[^::]+)/;
    case 'unknown':
      return /^(?<packageName>.+)$/;
  }

  return /^(?<packageName>.+)$/;
}

// TODO spy names?
export function getPackageNameFromStackTrace(
  spyName: SpyName,
  stackTrace: string
) {
  if (stackTrace.length === 0) {
    return stackTrace;
  }
  const regexp = spyToRegex(spyName);
  const fullStackGroups = stackTrace.match(regexp);
  if (fullStackGroups && fullStackGroups.groups) {
    return fullStackGroups.groups['packageName'] || '';
  }
  return stackTrace;
}

export function colorBasedOnPackageName(
  palette: FlamegraphPalette,
  name: string
) {
  const hash = murmurhashThree32GC(name, 0);
  const colorIndex = hash % palette.colors.length;
  const baseClr = palette.colors[colorIndex];
  if (!baseClr) {
    console.warn('Could not calculate color. Defaulting to the first one');
    // We assert to Color since the first position is always available
    return palette.colors[0];
  }

  return baseClr;
}

/**
 * NewDiffColor constructs a function that given a number from -100 to 100
 * it returns the color for that number in a linear scale
 * encoded in rgb
 */
export function NewDiffColor(
  props: Omit<FlamegraphPalette, 'colors'>
): (n: number) => Color {
  const { goodColor, neutralColor, badColor } = props;

  const color = scaleLinear()
    .domain([-100, 0, 100])
    // TODO types from DefinitelyTyped seem to mismatch
    .range([
      goodColor.rgb().toString(),
      neutralColor.rgb().toString(),
      badColor.rgb().toString(),
    ] as ShamefulAny);

  return (n: number) => {
    // convert to our Color object
    // since that's what users are expecting to use
    return Color(color(n).toString());
  };
}
