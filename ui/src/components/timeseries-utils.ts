export function toDisplayValue(raw: number, unit: string): number {
  if (unit === 'ns') return raw / 1e9;
  return raw;
}

export function niceMax(value: number): number {
  if (value <= 0) return 1;
  const exp = Math.floor(Math.log10(value));
  const mag = Math.pow(10, exp);
  const norm = value / mag;
  if (norm <= 1) return mag;
  if (norm <= 2) return 2 * mag;
  if (norm <= 5) return 5 * mag;
  return 10 * mag;
}

export function yAxisFormatter(displayMax: number): (v: number) => string {
  let divisor = 1,
    suffix = '';
  if (displayMax >= 1e9) {
    divisor = 1e9;
    suffix = 'G';
  } else if (displayMax >= 1e6) {
    divisor = 1e6;
    suffix = 'M';
  } else if (displayMax >= 1e3) {
    divisor = 1e3;
    suffix = 'k';
  } else if (displayMax < 1e-3 && displayMax > 0) {
    divisor = 1e-6;
    suffix = 'µ';
  } else if (displayMax < 1 && displayMax > 0) {
    divisor = 1e-3;
    suffix = 'm';
  }
  return (v: number) => {
    if (v === 0) return '0';
    return `${parseFloat((v / divisor).toPrecision(3))}${suffix}`;
  };
}

export function tickStepMs(durationMs: number): number {
  const m = 60_000,
    h = 3_600_000,
    d = 86_400_000;
  if (durationMs <= 15 * m) return m;
  if (durationMs <= 2 * h) return 5 * m;
  if (durationMs <= 4 * h) return 15 * m;
  if (durationMs <= 8 * h) return 30 * m;
  if (durationMs <= 12 * h) return h;
  if (durationMs <= d) return 2 * h;
  if (durationMs <= 7 * d) return 12 * h;
  return d;
}

export function formatTickTime(ts: number, stepMs: number): string {
  const d = new Date(ts);
  if (stepMs >= 86_400_000) {
    return d.toLocaleDateString(undefined, {
      month: 'numeric',
      day: 'numeric',
    });
  }
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}
