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

  describe('format & formatPrecise', () => {
    // TODO is this correct, since we have an enum?
    // unfortunately until we fully migrate to TS
    // we still need to check for a default value
    it('its constructor should default to DurationFormatter', () => {
      const df = getFormatter(80, 2, '' as any);

      expect(df.format(0.001, 100)).toBe('< 0.01  ');
      expect(df.formatPrecise(0.001, 100)).toBe('0.00001  ');
      expect(df.formatPrecise(0.1, 100)).toBe('0.001  ');
    });

    describe('DurationFormatter', () => {
      it('correctly formats duration when maxdur = 40', () => {
        const df = getFormatter(80, 2, 'samples');

        expect(df.format(0, 100)).toBe('0.00 seconds');
        expect(df.format(0.001, 100)).toBe('< 0.01 seconds');
        expect(df.format(100, 100)).toBe('1.00 second');
        expect(df.format(2000, 100)).toBe('20.00 seconds');
        expect(df.format(2012.3, 100)).toBe('20.12 seconds');
        expect(df.format(8000, 100)).toBe('80.00 seconds');
        expect(df.formatPrecise(0.001, 100)).toBe('0.00001 seconds');
      });

      it('correctly formats duration when maxdur = 80', () => {
        const df = getFormatter(160, 2, 'samples');

        expect(df.format(6000, 100)).toBe('1.00 minute');
        expect(df.format(100, 100)).toBe('0.02 minutes');
        expect(df.format(2000, 100)).toBe('0.33 minutes');
        expect(df.format(2012.3, 100)).toBe('0.34 minutes');
        expect(df.format(8000, 100)).toBe('1.33 minutes');
        expect(df.formatPrecise(1, 100)).toBe('0.00017 minutes');
      });

      it('correctly formats samples duration and return value without units', () => {
        const df = getFormatter(160, 2, 'samples');

        expect(df.suffix).toBe('minute');
        expect(df.format(6000, 100, false)).toBe('1.00');
        expect(df.format(100, 100, false)).toBe('0.02');
        expect(df.format(2000, 100, false)).toBe('0.33');
        expect(df.format(2012.3, 100, false)).toBe('0.34');
        expect(df.format(8000, 100, false)).toBe('1.33');
      });

      it('correctly formats trace samples', () => {
        const df = getFormatter(80, 2, 'trace_samples');

        expect(df.format(0.001, 100)).toBe('< 0.01 seconds');
        expect(df.format(100, 100)).toBe('1.00 second');
        expect(df.format(2000, 100)).toBe('20.00 seconds');
        expect(df.format(2012.3, 100)).toBe('20.12 seconds');
        expect(df.format(8000, 100)).toBe('80.00 seconds');
        expect(df.formatPrecise(0.001, 100)).toBe('0.00001 seconds');
      });

      it('correctly formats trace_samples duration when maxdur is less than second', () => {
        const df = getFormatter(10, 100, 'trace_samples');

        expect(df.format(55, 100)).toBe('550.00 ms');
        expect(df.format(100, 100)).toBe('1000.00 ms');
        expect(df.format(1.001, 100)).toBe('10.01 ms');
        expect(df.format(9999, 100)).toBe('99990.00 ms');
        expect(df.format(0.331, 100)).toBe('3.31 ms');
        expect(df.format(0.0001, 100)).toBe('< 0.01 ms');
        expect(df.formatPrecise(0.0001, 100)).toBe('0.001 ms');
      });

      it('correctly formats trace_samples duration when maxdur is less than ms', () => {
        const df = getFormatter(1, 10000, 'trace_samples');

        expect(df.format(0.012, 100)).toBe('120.00 μs');
        expect(df.format(0, 100)).toBe('0.00 μs');
        expect(df.format(0.0091, 100)).toBe('91.00 μs');
        expect(df.format(1.005199, 100)).toBe('10051.99 μs');
        expect(df.format(1.1, 100)).toBe('11000.00 μs');
        expect(df.format(0.000001, 100)).toBe('< 0.01 μs');
        expect(df.formatPrecise(0.0000001, 100)).toBe('0.001 μs');
      });

      it('correctly formats trace_samples duration when maxdur is hour', () => {
        const hour = 3600;
        let df = getFormatter(hour, 1, 'trace_samples');

        expect(df.format(0, 100)).toBe('0.00 hours');
        expect(df.format(hour * 100, 100)).toBe('1.00 hour');
        expect(df.format(0.6 * hour * 100, 100)).toBe('0.60 hours');
        expect(df.format(0.02 * hour * 100, 100)).toBe('0.02 hours');
        expect(df.format(0.001 * hour * 100, 100)).toBe('< 0.01 hours');
        expect(df.format(42.1 * hour * 100, 100)).toBe('42.10 hours');
        expect(df.formatPrecise(0.001 * hour * 100, 100)).toBe('0.001 hours');
      });

      it('correctly formats trace_samples duration when maxdur is day', () => {
        const day = 24 * 60 * 60;
        const df = getFormatter(day, 1, 'trace_samples');

        expect(df.format(day * 100, 100)).toBe('1.00 day');
        expect(df.format(12 * day * 100, 100)).toBe('12.00 days');
        expect(df.format(2.29 * day * 100, 100)).toBe('2.29 days');
        expect(df.format(0.11 * day * 100, 100)).toBe('0.11 days');
        expect(df.format(0.001 * day * 100, 100)).toBe('< 0.01 days');
        expect(df.formatPrecise(0.001 * day * 100, 100)).toBe('0.001 days');
      });

      it('correctly formats trace_samples duration when maxdur = month', () => {
        const month = 30 * 24 * 60 * 60;
        const df = getFormatter(month, 1, 'trace_samples');

        expect(df.format(month * 100, 100)).toBe('1.00 month');
        expect(df.format(44 * month * 100, 100)).toBe('44.00 months');
        expect(df.format(5.142 * month * 100, 100)).toBe('5.14 months');
        expect(df.format(0.88 * month * 100, 100)).toBe('0.88 months');
        expect(df.format(0.008 * month * 100, 100)).toBe('< 0.01 months');
        expect(df.formatPrecise(0.008 * month * 100, 100)).toBe('0.008 months');
      });

      it('correctly formats trace_samples duration when maxdur = year', () => {
        const year = 12 * 30 * 24 * 60 * 60;
        const df = getFormatter(year, 1, 'trace_samples');

        expect(df.format(year * 100, 100)).toBe('1.00 year');
        expect(df.format(12 * year * 100, 100)).toBe('12.00 years');
        expect(df.format(3.414 * year * 100, 100)).toBe('3.41 years');
        expect(df.format(0.12 * year * 100, 100)).toBe('0.12 years');
        expect(df.format(0.008 * year * 100, 100)).toBe('< 0.01 years');
        expect(df.formatPrecise(0.008 * year * 100, 100)).toBe('0.008 years');
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
            const f = getFormatter(maxObjects, sampleRate, 'objects');

            expect(f.format(samples, sampleRate)).toBe(expected);
          });
        }
      );
    });

    describe('ObjectsFormatter formatPrecise', () => {
      describe.each([
        [1, -1, '-1 '],
        [100_000, -1, '-0.001 K'],

        [1, 1, '1 '],
        [100_000, 1, '0.001 K'],
      ])(
        'new ObjectsFormatter(%i).format(%i, %i)',
        (maxObjects: number, samples: number, expected: string) => {
          it(`returns ${expected}`, () => {
            // sampleRate is not used
            const sampleRate = NaN;
            const f = getFormatter(maxObjects, sampleRate, 'objects');

            expect(f.formatPrecise(samples, sampleRate)).toBe(expected);
          });
        }
      );
    });

    describe('BytesFormatter', () => {
      describe.each([
        [1, -1, '-1.00 bytes'], // TODO is this correct?
        [1024, -1, '< 0.01 KB'],
        [1024 ** 2, -1, '< 0.01 MB'],
        [1024 ** 3, -1, '< 0.01 GB'],
        [1024 ** 4, -1, '< 0.01 TB'],

        [1, 1, '1.00 bytes'],
        [1024, 1, '< 0.01 KB'],
        [1024 ** 2, 1, '< 0.01 MB'],
        [1024 ** 3, 1, '< 0.01 GB'],
        [1024 ** 4, 1, '< 0.01 TB'],

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
            const f = getFormatter(maxObjects, sampleRate, 'bytes');

            expect(f.format(samples, sampleRate)).toBe(expected);
          });
        }
      );
    });

    describe('BytesFormatter', () => {
      describe.each([
        [1, -1, '-1 bytes'],
        [1024, -1, '-0.00098 KB'],
        [1024 ** 2, -10, '-0.00001 MB'],
        [1024 ** 3, -10000, '-0.00001 GB'],
        [1024 ** 4, -10000000, '-0.00001 TB'],

        [1, 1, '1 bytes'],
        [1024, 1, '0.00098 KB'],
        [1024 ** 2, 10, '0.00001 MB'],
        [1024 ** 3, 10000, '0.00001 GB'],
        [1024 ** 4, 10000000, '0.00001 TB'],
      ])(
        'new BytesFormatter(%i).format(%i, %i)',
        (maxObjects: number, samples: number, expected: string) => {
          it(`returns ${expected}`, () => {
            // sampleRate is not used
            const sampleRate = NaN;
            const f = getFormatter(maxObjects, sampleRate, 'bytes');

            expect(f.formatPrecise(samples, sampleRate)).toBe(expected);
          });
        }
      );
    });
  });
});
