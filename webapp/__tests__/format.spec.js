import {DurationFormater} from '../javascript/util/format';


describe('DurationFormater', () => {
  it('correctly formats duration', () => {
    const df = new DurationFormater(40);
    expect(df.format(0.00001)).toBe('< 0.01 seconds');
    expect(df.format(1)).toBe('1.00 second');
    expect(df.format(20)).toBe('20.00 seconds');
    expect(df.format(20.123)).toBe('20.12 seconds');
    expect(df.format(80)).toBe('80.00 seconds');
  });

  it('correctly formats duration', () => {
    const df = new DurationFormater(80);
    expect(df.format(60)).toBe('1.00 minute');
    expect(df.format(1)).toBe('0.02 minutes');
    expect(df.format(20)).toBe('0.33 minutes');
    expect(df.format(20.123)).toBe('0.34 minutes');
    expect(df.format(80)).toBe('1.33 minutes');
  });
});
