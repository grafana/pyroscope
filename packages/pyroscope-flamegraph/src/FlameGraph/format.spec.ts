/* eslint-disable no-restricted-properties */
import { numberWithCommas, getFormatter, Units } from './format';

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

  describe('format', () => {
    // TODO is this correct, since we have an enum?
    // unfortunately until we fully migrate to TS
    // we still need to check for a default value
    it('its constructor should default to DurationFormatter', () => {
      const df = getFormatter(80, 2, '' as any);

      expect(df.format(0.001, 100)).toBe('< 0.01 seconds');
    });

    describe('DurationFormatter', () => {
      it('correctly formats duration when maxdur = 40', () => {
        const df = getFormatter(80, 2, Units.Samples);

        expect(df.format(0.001, 100)).toBe('< 0.01 seconds');
        expect(df.format(100, 100)).toBe('1.00 second');
        expect(df.format(2000, 100)).toBe('20.00 seconds');
        expect(df.format(2012.3, 100)).toBe('20.12 seconds');
        expect(df.format(8000, 100)).toBe('80.00 seconds');
      });

      it('correctly formats duration when maxdur = 80', () => {
        const df = getFormatter(160, 2, Units.Samples);

        expect(df.format(6000, 100)).toBe('1.00 minute');
        expect(df.format(100, 100)).toBe('0.02 minutes');
        expect(df.format(2000, 100)).toBe('0.33 minutes');
        expect(df.format(2012.3, 100)).toBe('0.34 minutes');
        expect(df.format(8000, 100)).toBe('1.33 minutes');
      });
    });

    describe('ObjectsFormatter', () => {
      describe.each([
        [1, -1, '-1.00 '],
        [100_000, -1, '< 0.01 K'],
        [1_000_000, -1, '< 0.01 M'],
        [1_000_000_000, -1, '< 0.01 G'],
        [1_000_000_000_000, -1, '< 0.01 T'],
        [1_000_000_000_000_000, -1, '< 0.01 P'],

        [1, 1, '1.00 '],
        [100_000, 1, '< 0.01 K'],
        [1_000_000, 1, '< 0.01 M'],
        [1_000_000_000, 1, '< 0.01 G'],
        [1_000_000_000_000, 1, '< 0.01 T'],
        [1_000_000_000_000_000, 1, '< 0.01 P'],

        // if the tests here feel random, that's because they are
        // input and outputs were reproduced from real data
        [829449, 829449, '829.45 K'],
        [747270, 747270, '747.27 K'],
        [747270, 273208, '273.21 K'],
        [747270, 37257, '37.26 K'],
        [747270, 140789, '140.79 K'],
        [747270, 183646, '183.65 K'],
        [747270, 67736, '67.74 K'],
        [747270, 243513, '243.51 K'],
        [747270, 55297, '55.30 K'],
        [747270, 62261, '62.26 K'],
        [747270, 98304, '98.30 K'],
        [747270, 65536, '65.54 K'],

        [280614985, 124590057, '124.59 M'],
        [280614985, 15947382, '15.95 M'],
        [280614985, 15949534, '15.95 M'],
        [280614985, 23392042, '23.39 M'],
        [280614985, 100988801, '100.99 M'],
        [280614985, 280614985, '280.61 M'],
        [280614985, 30556974, '30.56 M'],
        [280614985, 51105740, '51.11 M'],
        [280614985, 92737376, '92.74 M'],

        [1536297877, 166358124, '0.17 G'],
        [1536297877, 94577307, '0.09 G'],
        [1536297877, 205971847, '0.21 G'],
        [1536297877, 245667926, '0.25 G'],
      ])(
        'new ObjectsFormatter(%i).format(%i, %i)',
        (maxObjects: number, samples: number, expected: string) => {
          it(`returns ${expected}`, () => {
            // sampleRate is not used
            const sampleRate = NaN;
            const f = getFormatter(maxObjects, sampleRate, Units.Objects);

            expect(f.format(samples, sampleRate)).toBe(expected);
          });
        }
      );
    });

    describe('BytesFormatter', () => {
      describe.each([
        [1, -1, '-1.00 bytes'], // TODO is this correct?
        [1024, -1, '< 0.01 KB'],
        [Math.pow(1024, 2), -1, '< 0.01 MB'],
        [Math.pow(1024, 3), -1, '< 0.01 GB'],
        [Math.pow(1024, 4), -1, '< 0.01 TB'],

        [1, 1, '1.00 bytes'],
        [1024, 1, '< 0.01 KB'],
        [Math.pow(1024, 2), 1, '< 0.01 MB'],
        [Math.pow(1024, 3), 1, '< 0.01 GB'],
        [Math.pow(1024, 4), 1, '< 0.01 TB'],

        // if the tests here feel random, that's because they are
        // input and outputs were reproduced from real data
        [338855357, 269094260, '256.63 MB'],
        [338855357, 21498656, '20.50 MB'],
        [33261774660, 2369569091, '2.21 GB'],
        [33261774660, 12110767522, '11.28 GB'],
      ])(
        'new BytesFormatter(%i).format(%i, %i)',
        (maxObjects: number, samples: number, expected: string) => {
          it(`returns ${expected}`, () => {
            // sampleRate is not used
            const sampleRate = NaN;
            const f = getFormatter(maxObjects, sampleRate, Units.Bytes);

            expect(f.format(samples, sampleRate)).toBe(expected);
          });
        }
      );
    });
  });
});
