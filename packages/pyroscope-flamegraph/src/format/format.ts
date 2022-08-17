/* eslint-disable max-classes-per-file */
import { Units } from '@pyroscope/models/src';
import _last from 'lodash/last';

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

export function getFormatter(max: number, sampleRate: number, unit: Units) {
  switch (unit) {
    case 'samples':
      return new DurationFormatter(max / sampleRate);
    case 'objects':
      return new ObjectsFormatter(max);
    case 'goroutines':
      return new ObjectsFormatter(max);
    case 'bytes':
      return new BytesFormatter(max);
    case 'lock_nanoseconds':
      return new NanosecondsFormatter(max);
    case 'lock_samples':
      return new ObjectsFormatter(max);
    case 'trace_samples':
      return new SubSecondDurationFormatter(max / sampleRate);
    default:
      console.warn(`Unsupported unit: '${unit}'. Defaulting to ''`);
      return new DurationFormatter(max / sampleRate, ' ');
  }
}

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
class DurationFormatter {
  divider = 1;

  suffix = 'second';

  durations: [number, string][] = [
    [60, 'minute'],
    [60, 'hour'],
    [24, 'day'],
    [30, 'month'],
    [12, 'year'],
  ];

  units = '';

  constructor(maxDur: number, units?: string) {
    this.units = units || '';
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

  format(samples: number, sampleRate: number): string {
    const n = samples / sampleRate / this.divider;
    let nStr = n.toFixed(2);

    if (n === 0) {
      nStr = '0.00';
    } else if (n >= 0 && n < 0.01) {
      nStr = '< 0.01';
    } else if (n <= 0 && n > -0.01) {
      nStr = '< 0.01';
    }

    return `${nStr} ${this.units || `${this.suffix}${n === 1 ? '' : 's'}`}`;
  }
}

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
class SubSecondDurationFormatter {
  divider = 1;

  suffix = 'second';

  durations: [number, string][] = [
    [60, 'minute'],
    [60, 'hour'],
    [24, 'day'],
    [30, 'month'],
    [12, 'year'],
  ];

  subSecondDurations: [number, string][] = [
    [1000, 'ms'],
    [1000, 'μs'],
  ];

  units = '';

  constructor(maxDur: number, units?: string) {
    this.units = units || '';
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

  format(samples: number, sampleRate: number): string {
    let n = samples / sampleRate / this.divider;

    if (n && !Number.isInteger(n) && this.divider === 1) {
      // n is float and we are in the seconds
      // eslint-disable-next-line no-plusplus
      for (let i = 0; i < this.subSecondDurations.length; i++) {
        const [multiplier, suffix] = this.subSecondDurations[i];
        // floating math is broken https://stackoverflow.com/questions/588004/is-floating-point-math-broken so we use this workaround
        n = Number((n * multiplier).toPrecision(15));
        if (Number.isInteger(n)) return `${n}.00 ${this.units || suffix}`;
      }
      const lastSubSecDuration = _last(this.subSecondDurations) as [
        number,
        string
      ];
      return `${n.toFixed(2)} ${this.units || `${lastSubSecDuration[1]}`}`;
    }

    const nStr = n.toFixed(2);
    return `${nStr} ${this.units || `${this.suffix}${n === 1 ? '' : 's'}`}`;
  }
}

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
class NanosecondsFormatter {
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

  format(samples: number) {
    const n = samples / 1000000000 / this.divider;
    let nStr = n.toFixed(2);

    if (n >= 0 && n < 0.01) {
      nStr = '< 0.01';
    } else if (n <= 0 && n > -0.01) {
      nStr = '< 0.01';
    }

    return `${nStr} ${this.suffix}${n === 1 ? '' : 's'}`;
  }
}

export class ObjectsFormatter {
  divider = 1;

  suffix = '';

  objects: [number, string][] = [
    [1000, 'K'],
    [1000, 'M'],
    [1000, 'G'],
    [1000, 'T'],
    [1000, 'P'],
  ];

  constructor(maxObjects: number) {
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
  }

  // TODO:
  // how to indicate that sampleRate doesn't matter?
  format(samples: number) {
    const n = samples / this.divider;
    let nStr = n.toFixed(2);

    if (n >= 0 && n < 0.01) {
      nStr = '< 0.01';
    } else if (n <= 0 && n > -0.01) {
      nStr = '< 0.01';
    }
    return `${nStr} ${this.suffix}`;
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

  constructor(maxBytes: number) {
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
  }

  format(samples: number) {
    const n = samples / this.divider;
    let nStr = n.toFixed(2);

    if (n >= 0 && n < 0.01) {
      nStr = '< 0.01';
    } else if (n <= 0 && n > -0.01) {
      nStr = '< 0.01';
    }

    return `${nStr} ${this.suffix}`;
  }
}
