export function numberWithCommas(x) {
  return x.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

const suffixes = [
  "K",
  "M",
  "G",
  "T",
];

export function shortNumber(x) {
  let suffix = '';

  for(var i = 0; x > 1000 && i < suffixes.length; i++) {
    suffix = suffixes[i];
    x /= 1000;
  }

  return Math.round(x).toString() + suffix;
}

export function formatPercent(ratio) {
  const percent = Math.round(10000 * ratio) / 100;
  return percent+'%';
}

const durations = [
  [60, "minute"],
  [60, "hour"],
  [24, "day"],
  [30, "month"],
  [12, "year"],
];

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
export class DurationFormatter {
  constructor(maxDur) {
    this.divider = 1;
    this.suffix = 'second';
    for(var i = 0; i < durations.length; i++) {
      if (maxDur >= durations[i][0]) {
        this.divider *= durations[i][0];
        maxDur /= durations[i][0];
        this.suffix = durations[i][1];
      } else {
        break;
      }
    }
  }

  format(samples, sampleRate) {
    let number = samples / sampleRate / this.divider;
    if (number >= 0 && number < 0.01) {
      number = '< 0.01';
    } else if (number <= 0 && number > -0.01) {
      number = '< 0.01';
    } else {
      number = number.toFixed(2);
    }
    return `${number} ${this.suffix}` + (number == 1 ? '' : 's');
  }
}


const bytes = [
  [1024, "KB"],
  [1024, "MB"],
  [1024, "GB"],
  [1024, "TB"],
  [1024, "PB"],
];

export class BytesFormatter {
  constructor(maxBytes) {
    this.divider = 1;
    this.suffix = 'bytes';
    for(var i = 0; i < bytes.length; i++) {
      if (maxBytes >= bytes[i][0]) {
        this.divider *= bytes[i][0];
        maxBytes /= bytes[i][0];
        this.suffix = bytes[i][1];
      } else {
        break;
      }
    }
  }

  format(samples, sampleRate) {
    let number = samples / this.divider;
    if (number >= 0 && number < 0.01) {
      number = '< 0.01';
    } else if (number <= 0 && number > -0.01) {
      number = '< 0.01';
    } else {
      number = number.toFixed(2);
    }
    return `${number} ${this.suffix}`;
  }
}

const objects = [
  [1000, "K"],
  [1000, "M"],
  [1000, "G"],
  [1000, "T"],
  [1000, "P"],
];

export class ObjectsFormatter {
  constructor(maxObjects) {
    this.divider = 1;
    this.suffix = '';
    for(var i = 0; i < objects.length; i++) {
      if (maxObjects >= objects[i][0]) {
        this.divider *= objects[i][0];
        maxObjects /= objects[i][0];
        this.suffix = objects[i][1];
      } else {
        break;
      }
    }
  }

  format(samples, sampleRate) {
    let number = samples / this.divider;
    if (number >= 0 && number < 0.01) {
      number = '< 0.01';
    } else if (number <= 0 && number > -0.01) {
      number = '< 0.01';
    } else {
      number = number.toFixed(2);
    }
    return `${number} ${this.suffix}`;
  }
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

export function getFormatter(max, sampleRate, units){
  switch (units) {
    case "samples":
      return new DurationFormatter(max / sampleRate);
    case "objects":
      return new ObjectsFormatter(max);
    case "bytes":
      return new BytesFormatter(max);
    default:
      return new DurationFormatter(max / sampleRate);
  }
}
