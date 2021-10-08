/* eslint-disable no-plusplus */
/* eslint-disable prefer-destructuring */
/* eslint-disable max-classes-per-file */
// export function numberWithCommas(x) {
//  return x.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
// }

const suffixes = ['K', 'M', 'G', 'T'];

export function shortNumber(x) {
  let suffix = '';

  for (let i = 0; x > 1000 && i < suffixes.length; i++) {
    suffix = suffixes[i];
    x /= 1000;
  }

  return Math.round(x).toString() + suffix;
}

export function ratioToPercent(ratio) {
  return Math.round(10000 * ratio) / 100;
}
export function formatPercent(ratio) {
  const percent = ratioToPercent(ratio);
  return `${percent}%`;
}

export function getPackageNameFromStackTrace(spyName, stackTrace) {
  // TODO: actually make sure these make sense and add tests
  const regexpLookup = {
    default: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
    dotnetspy: /^(?<packageName>.+)\.(.+)\.(.+)\(.*\)$/,
    ebpfspy: /^(?<packageName>.+)$/,
    gospy: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
    phpspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.php+)(?<line_info>.*)$/,
    pyspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/,
    rbspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.rb+)(?<line_info>.*)$/,
  };

  if (stackTrace.length === 0) {
    return stackTrace;
  }
  const regexp = regexpLookup[spyName] || regexpLookup.default;
  const fullStackGroups = stackTrace.match(regexp);
  if (fullStackGroups) {
    return fullStackGroups.groups.packageName;
  }
  return stackTrace;
}

export function getFormatter(max, sampleRate, units) {
  switch (units) {
    case 'samples':
      return new DurationFormatter(max / sampleRate);
    case 'objects':
      return new ObjectsFormatter(max);
    case 'bytes':
      return new BytesFormatter(max);
    default:
      return new DurationFormatter(max / sampleRate);
  }
}

export function percentDiff(leftPercent, rightPercent) {
  // difference between 2 percents
  // https://en.wikipedia.org/wiki/Relative_change_and_difference
  return ((rightPercent - leftPercent) / leftPercent) * 100;
}
