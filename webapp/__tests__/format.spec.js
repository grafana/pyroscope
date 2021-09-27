import { DurationFormatter } from '../javascript/util/format';

describe('DurationFormatter', () => {
  it('correctly formats duration', () => {
    const df = new DurationFormatter(40);
    expect(df.format(0.001, 100)).toBe('< 0.01 seconds');
    expect(df.format(100, 100)).toBe('1.00 second');
    expect(df.format(2000, 100)).toBe('20.00 seconds');
    expect(df.format(2012.3, 100)).toBe('20.12 seconds');
    expect(df.format(8000, 100)).toBe('80.00 seconds');
  });

  it('correctly formats duration', () => {
    const df = new DurationFormatter(80);
    expect(df.format(6000, 100)).toBe('1.00 minute');
    expect(df.format(100, 100)).toBe('0.02 minutes');
    expect(df.format(2000, 100)).toBe('0.33 minutes');
    expect(df.format(2012.3, 100)).toBe('0.34 minutes');
    expect(df.format(8000, 100)).toBe('1.33 minutes');
  });
});
