import { readableRange } from '@utils/formatDate';

describe('FormatDate', () => {
  describe('readableRange', () => {
    const cases = [
      ['now-5m', 'now', 'Last 5 minutes'],
      ['now-15m', 'now', 'Last 15 minutes'],
      ['now-1h', 'now', 'Last 1 hour'],
      ['now-24h', 'now', 'Last 24 hours'],
      ['now-1d', 'now', 'Last 1 day'],
      ['now-2d', 'now', 'Last 2 days'],
      ['now-30d', 'now', 'Last 30 days'],
      ['now-1M', 'now', 'Last 1 month'],
      ['now-6M', 'now', 'Last 6 months'],
      ['now-1y', 'now', 'Last 1 year'],
      ['now-2y', 'now', 'Last 2 years'],
      [1624278889, 1640090089, '2021-06-21 09:34 AM - 2021-12-21 09:34 AM'],
    ];

    test.each(cases)(
      'readableRange(%s, %s) should be %s',
      (from, until, expected) => {
        expect(readableRange(from, until)).toBe(expected);
      }
    );
  });
});
