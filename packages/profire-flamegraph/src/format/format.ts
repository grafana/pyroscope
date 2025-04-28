/* eslint-disable max-classes-per-file */
import { Units } from '@pyroscope/models/src/units';
import { time } from 'console';

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
  samples: number,
  divider: number,
  unitStr: string,
  withUnits: boolean,
  levels: [number, string][],
  precision = 2,
  withS = false
): string {
  if (isNaN(samples) || !isFinite(samples) || samples === 0) {
    return '0';
  }
  const n = samples / divider;
  const absN = Math.abs(n);
  if (absN < 0.01) {
    const curUnitLevel = levels.findIndex(([_, name]) => name === unitStr);
    if (curUnitLevel === -1 || curUnitLevel === 0) {
      return '< 0.01';
    }
    for (let i = curUnitLevel - 1; i >= 0; i--) {
      const [smallerLevel, smallerUnit] = levels[i];
      const converted = samples / smallerLevel;
      if (Math.abs(converted) >= 0.01) {
        const numStr = prettyNum(converted.toFixed(precision));
        return `${numStr} ${smallerUnit}${
          withS && !smallerUnit.endsWith('s') ? 's' : ''
        }`;
      }
    }
    return '< 0.01';
  }

  const cleanNumStr = prettyNum(n.toFixed(precision));

  const res = withUnits
    ? `${cleanNumStr} ${unitStr}${
        // eslint-disable-next-line no-nested-ternary
        withS && !unitStr.endsWith('s') ? (n === 1 ? '' : 's') : ''
      }`
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
  if (unitLevel > 0 && unitLevel <= levels.length) {
    const [div, label] = levels[unitLevel - 1];
    state.divider = div;
    state.suffix = label;
  } else {
    defaultFn();
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

const timeLevels: [number, string][] = [
  [1, 'ns'],
  [1e3, 'us'],
  [1e6, 'ms'],
  [1e9, 'sec'],
  [60 * 1e9, 'min'],
  [60 * 60 * 1e9, 'hour'],
];

const bytesLevels: [number, string][] = [
  [1, 'B'],
  [1024, 'KB'],
  [1024 ** 2, 'MB'],
  [1024 ** 3, 'GB'],
  [1024 ** 4, 'TB'],
  [1024 ** 5, 'PB'],
];

const objectLevels: [number, string][] = [
  [1, ''],
  [1000, 'K'],
  [1000 ** 2, 'M'],
  [1000 ** 3, 'G'],
  [1000 ** 4, 'T'],
  [1000 ** 5, 'P'],
];

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
export class DurationFormatter extends Formatter {
  divider = 1;

  enableSubsecondPrecision = false;

  suffix = 'sec';

  durations: [number, string][] = [
    [60, 'min'],
    [60, 'hour'],
    [24, 'day'],
    [30, 'month'],
    [12, 'year'],
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
      this.durations = [[1000, 'ms'], [1000, 'sec'], ...this.durations];
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
    getLevelOrDefault(unitLevel, timeLevels, f, tmpState);
    if (unitLevel !== 0) {
      this.divider = tmpState.divider;
      this.suffix = tmpState.suffix;
    }
  }

  format(samples: number, sampleRate: number, withUnits = true): string {
    if (this.enableSubsecondPrecision) {
      sampleRate /= 1e6;
    }
    return formatNumber(
      samples,
      sampleRate * this.divider,
      this.suffix,
      withUnits,
      timeLevels,
      2,
      true
    );
  }

  formatPrecise(samples: number, sampleRate: number) {
    if (this.enableSubsecondPrecision) {
      sampleRate /= 1e6;
    }
    return formatNumber(
      samples,
      sampleRate / this.divider,
      this.suffix,
      true,
      timeLevels,
      5,
      true
    );
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
    return formatNumber(
      samples,
      1e9 * this.divider,
      this.suffix,
      withUnits,
      timeLevels
    );
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
    getLevelOrDefault(unitLevel, objectLevels, f, tmpState);
    if (unitLevel !== 0) {
      this.divider = tmpState.divider;
      this.suffix = tmpState.suffix;
    }
  }

  format(samples: number, sampleRate?: number, withUnits = true) {
    return formatNumber(
      samples,
      this.divider,
      this.suffix,
      withUnits,
      objectLevels
    );
  }

  formatPrecise(samples: number, withUnits = true) {
    return formatNumber(
      samples,
      this.divider,
      this.suffix,
      withUnits,
      objectLevels,
      5
    );
  }
}

export class BytesFormatter {
  divider = 1;

  suffix = 'bytes';

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
    getLevelOrDefault(unitLevel, bytesLevels, f, tmpState);
    if (unitLevel !== 0) {
      this.divider = tmpState.divider;
      this.suffix = tmpState.suffix;
    }
  }

  format(samples: number, sampleRate?: number, withUnits = true) {
    return formatNumber(
      samples,
      this.divider,
      this.suffix,
      withUnits,
      bytesLevels,
      2
    );
  }

  formatPrecise(samples: number, withUnits = true) {
    return formatNumber(
      samples,
      this.divider,
      this.suffix,
      withUnits,
      bytesLevels,
      5
    );
  }
}
