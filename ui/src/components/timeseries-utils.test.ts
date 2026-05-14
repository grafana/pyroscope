import { describe, it, expect } from 'vitest';
import {
  toDisplayValue,
  niceMax,
  yAxisFormatter,
  tickStepMs,
  formatTickTime,
} from './timeseries-utils';

const MINUTE_MS = 60_000;
const HOUR_MS = 3_600_000;
const DAY_MS = 86_400_000;

describe('toDisplayValue', () => {
  it('divides nanoseconds by 1e9', () => {
    expect(toDisplayValue(1_000_000_000, 'ns')).toBe(1);
    expect(toDisplayValue(500_000_000, 'ns')).toBe(0.5);
  });

  it('passes other units through unchanged', () => {
    expect(toDisplayValue(42, 'bytes')).toBe(42);
    expect(toDisplayValue(42, 'count')).toBe(42);
  });
});

describe('niceMax', () => {
  it('returns 1 for zero or negative input', () => {
    expect(niceMax(0)).toBe(1);
    expect(niceMax(-5)).toBe(1);
  });

  it('rounds up to the nearest 1, 2, 5, or 10 magnitude', () => {
    expect(niceMax(1)).toBe(1);
    expect(niceMax(1.5)).toBe(2);
    expect(niceMax(3)).toBe(5);
    expect(niceMax(7)).toBe(10);
    expect(niceMax(150)).toBe(200);
    expect(niceMax(300)).toBe(500);
    expect(niceMax(800)).toBe(1000);
  });

  it('handles large values', () => {
    expect(niceMax(1_200_000)).toBe(2_000_000);
    expect(niceMax(9_999_999)).toBe(10_000_000);
  });
});

describe('yAxisFormatter', () => {
  it('formats zero as "0" regardless of scale', () => {
    expect(yAxisFormatter(1000)(0)).toBe('0');
    expect(yAxisFormatter(1e9)(0)).toBe('0');
  });

  it('uses G suffix for values >= 1e9', () => {
    const fmt = yAxisFormatter(2e9);
    expect(fmt(1e9)).toBe('1G');
    expect(fmt(1.5e9)).toBe('1.5G');
  });

  it('uses M suffix for values >= 1e6', () => {
    const fmt = yAxisFormatter(5e6);
    expect(fmt(1e6)).toBe('1M');
  });

  it('uses k suffix for values >= 1e3', () => {
    const fmt = yAxisFormatter(2000);
    expect(fmt(1000)).toBe('1k');
    expect(fmt(1500)).toBe('1.5k');
  });

  it('formats sub-1 values without suffix', () => {
    const fmt = yAxisFormatter(0.5);
    expect(fmt(0.5)).toBe('500m');
  });

  it('formats plain values with no suffix', () => {
    const fmt = yAxisFormatter(100);
    expect(fmt(50)).toBe('50');
    expect(fmt(100)).toBe('100');
  });
});

describe('tickStepMs', () => {
  it('uses 1-minute steps for durations up to 15 minutes', () => {
    expect(tickStepMs(5 * MINUTE_MS)).toBe(MINUTE_MS);
    expect(tickStepMs(15 * MINUTE_MS)).toBe(MINUTE_MS);
  });

  it('uses 5-minute steps for durations up to 2 hours', () => {
    expect(tickStepMs(30 * MINUTE_MS)).toBe(5 * MINUTE_MS);
    expect(tickStepMs(2 * HOUR_MS)).toBe(5 * MINUTE_MS);
  });

  it('uses 15-minute steps for durations up to 4 hours', () => {
    expect(tickStepMs(3 * HOUR_MS)).toBe(15 * MINUTE_MS);
    expect(tickStepMs(4 * HOUR_MS)).toBe(15 * MINUTE_MS);
  });

  it('uses 30-minute steps for durations up to 8 hours', () => {
    expect(tickStepMs(6 * HOUR_MS)).toBe(30 * MINUTE_MS);
    expect(tickStepMs(8 * HOUR_MS)).toBe(30 * MINUTE_MS);
  });

  it('uses 1-hour steps for durations up to 12 hours', () => {
    expect(tickStepMs(10 * HOUR_MS)).toBe(HOUR_MS);
    expect(tickStepMs(12 * HOUR_MS)).toBe(HOUR_MS);
  });

  it('uses 2-hour steps for durations up to 1 day', () => {
    expect(tickStepMs(18 * HOUR_MS)).toBe(2 * HOUR_MS);
    expect(tickStepMs(DAY_MS)).toBe(2 * HOUR_MS);
  });

  it('uses 12-hour steps for durations up to 7 days', () => {
    expect(tickStepMs(3 * DAY_MS)).toBe(12 * HOUR_MS);
    expect(tickStepMs(7 * DAY_MS)).toBe(12 * HOUR_MS);
  });

  it('uses 1-day steps for durations beyond 7 days', () => {
    expect(tickStepMs(10 * DAY_MS)).toBe(DAY_MS);
    expect(tickStepMs(30 * DAY_MS)).toBe(DAY_MS);
  });
});

describe('formatTickTime', () => {
  // 2024-03-15 14:30:00 UTC
  const ts = Date.UTC(2024, 2, 15, 14, 30, 0);

  it('formats as HH:MM for sub-day steps', () => {
    const result = formatTickTime(ts, HOUR_MS);
    // Hours depend on local timezone, so just verify the HH:MM pattern
    expect(result).toMatch(/^\d{2}:\d{2}$/);
  });

  it('formats as a date string for day-level steps', () => {
    const result = formatTickTime(ts, DAY_MS);
    // toLocaleDateString with month/day numeric — just check it's not HH:MM
    expect(result).not.toMatch(/^\d{2}:\d{2}$/);
    expect(result.length).toBeGreaterThan(0);
  });
});
