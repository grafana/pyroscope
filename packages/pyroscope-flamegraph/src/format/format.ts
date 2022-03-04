/* eslint-disable max-classes-per-file */
import { Units } from '@pyroscope/models';

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
    case 'bytes':
      return new BytesFormatter(max);
    default:
      console.warn(`Unsupported unit: '${unit}'. Defaulting to 'samples'`);
      return new DurationFormatter(max / sampleRate);
  }
}

// this is a class and not a function because we can save some time by
//   precalculating divider and suffix and not doing it on each iteration
class DurationFormatter {
  divider = 1;

  suffix: string = 'second';

  durations: [number, string][] = [
    [60, 'minute'],
    [60, 'hour'],
    [24, 'day'],
    [30, 'month'],
    [12, 'year'],
  ];

  constructor(maxDur: number) {
    // eslint-disable-next-line no-plusplus
    for (let i = 0; i < this.durations.length; i++) {
      if (maxDur >= this.durations[i][0]) {
        this.divider *= this.durations[i][0];
        maxDur /= this.durations[i][0];
        // eslint-disable-next-line prefer-destructuring
        this.suffix = this.durations[i][1];
      } else {
        break;
      }
    }
  }

  format(samples: number, sampleRate: number) {
    const n = samples / sampleRate / this.divider;
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
      if (maxObjects >= this.objects[i][0]) {
        this.divider *= this.objects[i][0];
        maxObjects /= this.objects[i][0];
        // eslint-disable-next-line prefer-destructuring
        this.suffix = this.objects[i][1];
      } else {
        break;
      }
    }
  }

  // TODO:
  // how to indicate that sampleRate doesn't matter?
  format(samples: number, sampleRate: number) {
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
      if (maxBytes >= this.bytes[i][0]) {
        this.divider *= this.bytes[i][0];
        maxBytes /= this.bytes[i][0];
        // eslint-disable-next-line prefer-destructuring
        this.suffix = this.bytes[i][1];
      } else {
        break;
      }
    }
  }

  format(samples: number, sampleRate: number) {
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
