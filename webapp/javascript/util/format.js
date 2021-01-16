export function numberWithCommas(x) {
  return x.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

import humanizeDuration from "humanize-duration";

const suffixes = [
  "K",
  "M",
  "G",
  "T",
]

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

const shortEnglishHumanizer = humanizeDuration.humanizer({
  language: "shortEn",
  languages: {
    shortEn: {
      y: () => "y",
      mo: () => "mo",
      w: () => "w",
      d: () => "d",
      h: () => "h",
      m: () => "m",
      s: () => "s",
      ms: () => "ms",
    },
  },
});


export function formatDuration(v, sampleRate) {
  const rounded = v * (1 / sampleRate) * 1000;
  return humanizeDuration(rounded, { largest: 1, maxDecimalPoints: 2 });
}

export function formatDurationLong(v, sampleRate) {
  const rounded = v * (1 / sampleRate) * 1000;
  return humanizeDuration(rounded, { maxDecimalPoints: 2 });
}
