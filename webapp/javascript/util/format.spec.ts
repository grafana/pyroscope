import { numberWithCommas, getFormatter } from './format';

describe('format', () => {
  describe.each([
    [0, '0'],
    [1_000, '1,000'],
    [1_000_000, '1,000,000'],
    [1_000_000_000, '1,000,000,000'],
    [-1_000, '-1,000'],
    [-1_000_000, '-1,000,000'],
    [-1_000_000_000, '-1,000,000,000'],
  ])('.numberWithCommas(%i)', (a: number, expected) => {
    it(`returns ${expected}`, () => {
      expect(numberWithCommas(a)).toBe(expected);
    });
  });

  //  describe('numberWithCommas()', () => {
  //
  //  });
  //
  //
  // TODO: don't export duration formatter
  // let us interact strictly via the constructor
  describe('format', () => {
    describe('DurationFormatter', () => {
      it('correctly formats duration when maxdur = 40', () => {
        const df = getFormatter(80, 2, 'samples');

        expect(df.format(0.001, 100)).toBe('< 0.01 seconds');
        expect(df.format(100, 100)).toBe('1.00 second');
        expect(df.format(2000, 100)).toBe('20.00 seconds');
        expect(df.format(2012.3, 100)).toBe('20.12 seconds');
        expect(df.format(8000, 100)).toBe('80.00 seconds');
      });

      it('correctly formats duration when maxdur = 80', () => {
        const df = getFormatter(160, 2, 'samples');

        expect(df.format(6000, 100)).toBe('1.00 minute');
        expect(df.format(100, 100)).toBe('0.02 minutes');
        expect(df.format(2000, 100)).toBe('0.33 minutes');
        expect(df.format(2012.3, 100)).toBe('0.34 minutes');
        expect(df.format(8000, 100)).toBe('1.33 minutes');
      });
    });
  });
});
