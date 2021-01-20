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
export class DurationFormater {
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

  format(seconds) {
    let number = seconds / this.divider;
    if (number < 0.01) {
      number = '< 0.01';
    } else {
      number = number.toFixed(2);
    }
    return `${number} ${this.suffix}` + (number == 1 ? '' : 's');
  }
}
