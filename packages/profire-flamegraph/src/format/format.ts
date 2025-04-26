/* eslint-disable max-classes-per-file */
import { Units } from '@pyroscope/models/src/units';

export function getUnitAbbreviation(unit: string): string {
  switch (unit.toLowerCase()) {
    case 'second':
      return 'sec';
    case 'minute':
      return 'min';
    case 'hour':
      return 'hr';
    case 'day':
      return 'day';
    case 'month':
      return 'mon';
    case 'year':
      return 'year';
    default:
      return unit;
  }
}

export function prettyNum(nStr: string): string {
  if (!nStr.includes('.')) {
    return nStr;
  }
  nStr = nStr.replace(/\.?0+$/, '');
  return nStr;
}

export function formatNumber(
  n: number,
  unitStr: string,
  withUnits: boolean,
  precision = 2,
  withS = false
): string {
  if (isNaN(n) || !isFinite(n) || n === 0) {
    return '0';
  }
  if (n >= 0 && n < 0.01) {
    return '< 0.01';
  }
  if (n <= 0 && n > -0.01) {
    return '< 0.01';
  }

  const cleanNumStr = prettyNum(n.toFixed(precision));

  // eslint-disable-next-line no-nested-ternary
  const res = withUnits
    ? withS
      ? `${cleanNumStr} ${unitStr}${n === 1 ? '' : 's'}`
      : `${cleanNumStr} ${unitStr}`
    : cleanNumStr;
  return res;
}

export function numberWithCommas(x: number): string {
  return x.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export function formatPercent(ratio: number) {
  const percent = ratioToPercent(ratio);
  return `${percent}%`;
}

export function ratioToPercent(ratio: number) {
  return Math.round(10000 * ratio) / 100;
}

export function diffPercent(leftPercent: number, rightPercent: number): number {
  // difference between 2 percents
  // https://en.wikipedia.org/wiki/Relative_change_and_difference
  return ((rightPercent - leftPercent) / leftPercent) * 100;
}

export function getLevelOrDefault(
  unitLevel: number,
  levels: [number, string][],
  defaultFn: () => void,
  state: { divider: number; suffix: string }
) {
  switch (unitLevel) {
    case 1: {
      const [div, label] = levels[0];
      state.divider = div;
      state.suffix = label;
      break;
    }
    case 2: {
      const [div, label] = levels[1];
      state.divider = div;
      state.suffix = label;
      break;
    }
    case 3: {
      const [div, label] = levels[2];
      state.divider = div;
      state.suffix = label;
      break;
    }
    case 0:
    default: {
      defaultFn();
    }
  }
}

abstract class Formatter {
  abstract format(
    samples: number,
    sampleRate?: number,
    withUnits?: boolean
  ): string;
}

export function getFormatter(
  max: number,
  sampleRate: number,
  unit: Units,
  unitLevel: number,
  unitStr = ''
) {
  switch (unit) {
    case 'samples':
      return new DurationFormatter(max / sampleRate, unitLevel);
    case 'objects':
      return new ObjectsFormatter(max, unitLevel);
    case 'goroutines':
      return new ObjectsFormatter(max, unitLevel);
    case 'bytes':
      return new BytesFormatter(max, unitLevel);
    case 'lock_nanoseconds':
      return new NanosecondsFormatter(max);
    case 'lock_samples':
      return new BytesFormatter(max, unitLevel);
    case 'trace_samples':
      return new DurationFormatter(max / sampleRate, unitLevel, unitStr, true);
    case 'exceptions':
      return new BytesFormatter(max, unitLevel);
    case 'set':
      return new DurationFormatter(max / sampleRate, unitLevel, unitStr, true);
    default:
      console.warn(`Unsupported unit: '${unit}'. Defaulting to '${unitStr}'`);
      return new DurationFormatter(max / sampleRate, unitLevel, unitStr);
  }
}

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
export class DurationFormatter extends Formatter {
  divider = 1;

  enableSubsecondPrecision = false;

  suffix = 'second';

  durations: [number, string][] = [
    [60, 'minute'],
    [60, 'hour'],
    [24, 'day'],
    [30, 'month'],
    [12, 'year'],
  ];

  levels: [number, string][] = [
    [1, 'second'],
    [60, 'minite'],
    [60 * 60, 'hour'],
  ];

  units = '';

  constructor(
    maxDur: number,
    unitLevel: number,
    units?: string,
    enableSubsecondPrecision?: boolean
  ) {
    super();
    if (enableSubsecondPrecision) {
      this.enableSubsecondPrecision = enableSubsecondPrecision;
      this.durations = [[1000, 'ms'], [1000, 'second'], ...this.durations];
      this.suffix = `μs`;
      maxDur *= 1e6; // Converting seconds to μs
    }
    this.units = units || '';

    const f = () => {
      for (let i = 0; i < this.durations.length; i++) {
        const level = this.durations[i];
        if (!level) {
          console.warn('Could not calculate level');
          break;
        }

        if (maxDur >= level[0]) {
          this.divider *= level[0];
          maxDur /= level[0];
          // eslint-disable-next-line prefer-destructuring
          this.suffix = level[1];
        } else {
          break;
        }
      }
    };

    const tmpState = { divider: this.divider, suffix: this.suffix };
    getLevelOrDefault(unitLevel, this.levels, f, tmpState);
    if (unitLevel !== 0) {
      this.divider = tmpState.divider;
      this.suffix = tmpState.suffix;
    }
  }

  format(samples: number, sampleRate: number, withUnits = true): string {
    if (this.enableSubsecondPrecision) {
      sampleRate /= 1e6;
    }
    const n = samples / sampleRate / this.divider;
    return formatNumber(n, this.suffix, withUnits, 2, true);
  }

  formatPrecise(samples: number, sampleRate: number) {
    if (this.enableSubsecondPrecision) {
      sampleRate /= 1e6;
    }
    const n = samples / sampleRate / this.divider;

    return formatNumber(n, this.suffix, true, 5, true);
  }
}

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
export class NanosecondsFormatter extends Formatter {
  divider = 1;

  multiplier = 1;

  suffix = 'second';

  durations: [number, string][] = [
    [60, 'minute'],
    [60, 'hour'],
    [24, 'day'],
    [30, 'month'],
    [12, 'year'],
  ];

  constructor(maxDur: number) {
    super();
    maxDur /= 1000000000;
    // eslint-disable-next-line no-plusplus
    for (let i = 0; i < this.durations.length; i++) {
      const level = this.durations[i];
      if (!level) {
        console.warn('Could not calculate level');
        break;
      }

      if (maxDur >= level[0]) {
        this.divider *= level[0];
        maxDur /= level[0];
        // eslint-disable-next-line prefer-destructuring
        this.suffix = level[1];
      } else {
        break;
      }
    }
  }

  format(samples: number, sampleRate?, withUnits = true) {
    const n = samples / 1000000000 / this.divider;
    return formatNumber(n, this.suffix, withUnits);
  }

  formatPrecise(samples: number) {
    const n = samples / 1000000000 / this.divider;
    return `${parseFloat(n.toFixed(5))} ${this.suffix}${n === 1 ? '' : 's'}`;
  }
}

export class ObjectsFormatter extends Formatter {
  divider = 1;

  suffix = '';

  objects: [number, string][] = [
    [1000, 'K'],
    [1000, 'M'],
    [1000, 'G'],
    [1000, 'T'],
    [1000, 'P'],
  ];

  levels: [number, string][] = [
    [1000, 'K'],
    [1000 ** 2, 'M'],
    [1000 ** 3, 'G'],
  ];

  constructor(maxObjects: number, unitLevel: number) {
    super();
    const f = () => {
      // eslint-disable-next-line no-plusplus
      for (let i = 0; i < this.objects.length; i++) {
        const level = this.objects[i];
        if (!level) {
          console.warn('Could not calculate level');
          break;
        }

        if (maxObjects >= level[0]) {
          this.divider *= level[0];
          maxObjects /= level[0];
          // eslint-disable-next-line prefer-destructuring
          this.suffix = level[1];
        } else {
          break;
        }
      }
    };

    const tmpState = { divider: this.divider, suffix: this.suffix };
    getLevelOrDefault(unitLevel, this.levels, f, tmpState);
    if (unitLevel !== 0) {
      this.divider = tmpState.divider;
      this.suffix = tmpState.suffix;
    }
  }

  format(samples: number, sampleRate?: number, withUnits = true) {
    const n = samples / this.divider;
    return formatNumber(n, this.suffix, withUnits);
  }

  formatPrecise(samples: number, withUnits = true) {
    const n = samples / this.divider;
    return formatNumber(n, this.suffix, withUnits, 5);
  }
}

export class BytesFormatter {
  divider = 1;

  suffix = 'bytes';

  levels: [number, string][] = [
    [1024, 'KB'],
    [1024 ** 2, 'MB'],
    [1024 ** 3, 'GB'],
  ];

  bytes: [number, string][] = [
    [1024, 'KB'],
    [1024, 'MB'],
    [1024, 'GB'],
    [1024, 'TB'],
    [1024, 'PB'],
  ];

  constructor(maxBytes: number, unitLevel: number) {
    const f = () => {
      // eslint-disable-next-line no-plusplus
      for (let i = 0; i < this.bytes.length; i++) {
        const level = this.bytes[i];
        if (!level) {
          console.warn('Could not calculate level');
          break;
        }

        if (maxBytes >= level[0]) {
          this.divider *= level[0];
          maxBytes /= level[0];

          // eslint-disable-next-line prefer-destructuring
          const suffix = level[1];
          if (!suffix) {
            console.warn('Could not calculate suffix');
            this.suffix = '';
          } else {
            this.suffix = suffix;
          }
        } else {
          break;
        }
      }
    };

    const tmpState = { divider: this.divider, suffix: this.suffix };
    getLevelOrDefault(unitLevel, this.levels, f, tmpState);
    if (unitLevel !== 0) {
      this.divider = tmpState.divider;
      this.suffix = tmpState.suffix;
    }
  }

  format(samples: number, sampleRate?: number, withUnits = true) {
    const n = samples / this.divider;
    return formatNumber(n, this.suffix, withUnits, 2);
  }

  formatPrecise(samples: number, withUnits = true) {
    const n = samples / this.divider;
    return formatNumber(n, this.suffix, withUnits, 5);
  }
}
