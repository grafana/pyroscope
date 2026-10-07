import { describe, it } from 'node:test';
import { strict as assert } from 'node:assert';

import {
  toDisplayValue,
  niceMax,
  yAxisFormatter,
  parseRangeMs,
  tickStepMs,
  formatTickTime,
} from './timeseries-utils.ts';

const MINUTE_MS = 60_000;
const HOUR_MS = 3_600_000;
const DAY_MS = 86_400_000;

describe('toDisplayValue', () => {
  it('divides nanoseconds by 1e9', () => {
    assert.equal(toDisplayValue(1_000_000_000, 'ns'), 1);
    assert.equal(toDisplayValue(500_000_000, 'ns'), 0.5);
  });

  it('passes other units through unchanged', () => {
    assert.equal(toDisplayValue(42, 'bytes'), 42);
    assert.equal(toDisplayValue(42, 'count'), 42);
  });
});

describe('niceMax', () => {
  it('returns 1 for zero or negative input', () => {
    assert.equal(niceMax(0), 1);
    assert.equal(niceMax(-5), 1);
  });

  it('rounds up to the nearest 1, 2, 5, or 10 magnitude', () => {
    assert.equal(niceMax(1), 1);
    assert.equal(niceMax(1.5), 2);
    assert.equal(niceMax(3), 5);
    assert.equal(niceMax(7), 10);
    assert.equal(niceMax(150), 200);
    assert.equal(niceMax(300), 500);
    assert.equal(niceMax(800), 1000);
  });

  it('handles large values', () => {
    assert.equal(niceMax(1_200_000), 2_000_000);
    assert.equal(niceMax(9_999_999), 10_000_000);
  });
});

describe('yAxisFormatter', () => {
  it('formats zero as "0" regardless of scale', () => {
    assert.equal(yAxisFormatter(1000)(0), '0');
    assert.equal(yAxisFormatter(1e9)(0), '0');
  });

  it('uses G suffix for values >= 1e9', () => {
    const fmt = yAxisFormatter(2e9);
    assert.equal(fmt(1e9), '1G');
    assert.equal(fmt(1.5e9), '1.5G');
  });

  it('uses M suffix for values >= 1e6', () => {
    const fmt = yAxisFormatter(5e6);
    assert.equal(fmt(1e6), '1M');
  });

  it('uses k suffix for values >= 1e3', () => {
    const fmt = yAxisFormatter(2000);
    assert.equal(fmt(1000), '1k');
    assert.equal(fmt(1500), '1.5k');
  });

  it('formats sub-1 values without suffix', () => {
    const fmt = yAxisFormatter(0.5);
    assert.equal(fmt(0.5), '500m');
  });

  it('formats plain values with no suffix', () => {
    const fmt = yAxisFormatter(100);
    assert.equal(fmt(50), '50');
    assert.equal(fmt(100), '100');
  });
});

describe('parseRangeMs', () => {
  it('parses minutes', () => {
    assert.equal(parseRangeMs('now-5m'), 5 * MINUTE_MS);
    assert.equal(parseRangeMs('now-30m'), 30 * MINUTE_MS);
  });

  it('parses hours', () => {
    assert.equal(parseRangeMs('now-1h'), HOUR_MS);
    assert.equal(parseRangeMs('now-6h'), 6 * HOUR_MS);
  });

  it('parses days', () => {
    assert.equal(parseRangeMs('now-1d'), DAY_MS);
    assert.equal(parseRangeMs('now-7d'), 7 * DAY_MS);
  });

  it('falls back to 1 hour for unrecognized formats', () => {
    assert.equal(parseRangeMs('last-5m'), HOUR_MS);
    assert.equal(parseRangeMs(''), HOUR_MS);
    assert.equal(parseRangeMs('now-5x'), HOUR_MS);
  });
});

describe('tickStepMs', () => {
  it('uses 1-minute steps for durations up to 15 minutes', () => {
    assert.equal(tickStepMs(5 * MINUTE_MS), MINUTE_MS);
    assert.equal(tickStepMs(15 * MINUTE_MS), MINUTE_MS);
  });

  it('uses 5-minute steps for durations up to 2 hours', () => {
    assert.equal(tickStepMs(30 * MINUTE_MS), 5 * MINUTE_MS);
    assert.equal(tickStepMs(2 * HOUR_MS), 5 * MINUTE_MS);
  });

  it('uses 15-minute steps for durations up to 4 hours', () => {
    assert.equal(tickStepMs(3 * HOUR_MS), 15 * MINUTE_MS);
    assert.equal(tickStepMs(4 * HOUR_MS), 15 * MINUTE_MS);
  });

  it('uses 30-minute steps for durations up to 8 hours', () => {
    assert.equal(tickStepMs(6 * HOUR_MS), 30 * MINUTE_MS);
    assert.equal(tickStepMs(8 * HOUR_MS), 30 * MINUTE_MS);
  });

  it('uses 1-hour steps for durations up to 12 hours', () => {
    assert.equal(tickStepMs(10 * HOUR_MS), HOUR_MS);
    assert.equal(tickStepMs(12 * HOUR_MS), HOUR_MS);
  });

  it('uses 2-hour steps for durations up to 1 day', () => {
    assert.equal(tickStepMs(18 * HOUR_MS), 2 * HOUR_MS);
    assert.equal(tickStepMs(DAY_MS), 2 * HOUR_MS);
  });

  it('uses 12-hour steps for durations up to 7 days', () => {
    assert.equal(tickStepMs(3 * DAY_MS), 12 * HOUR_MS);
    assert.equal(tickStepMs(7 * DAY_MS), 12 * HOUR_MS);
  });

  it('uses 1-day steps for durations beyond 7 days', () => {
    assert.equal(tickStepMs(10 * DAY_MS), DAY_MS);
    assert.equal(tickStepMs(30 * DAY_MS), DAY_MS);
  });
});

describe('formatTickTime', () => {
  // 2024-03-15 14:30:00 UTC
  const ts = Date.UTC(2024, 2, 15, 14, 30, 0);

  it('formats as HH:MM for sub-day steps', () => {
    const result = formatTickTime(ts, HOUR_MS);
    // Hours depend on local timezone, so just verify the HH:MM pattern
    assert.match(result, /^\d{2}:\d{2}$/);
  });

  it('formats as a date string for day-level steps', () => {
    const result = formatTickTime(ts, DAY_MS);
    // toLocaleDateString with month/day numeric — just check it's not HH:MM
    assert.doesNotMatch(result, /^\d{2}:\d{2}$/);
    assert.ok(result.length > 0);
  });
});
