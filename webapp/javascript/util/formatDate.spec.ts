import { readableRange, formatAsOBject } from '@utils/formatDate';
import timezoneMock from 'timezone-mock';

describe('FormatDate', () => {
  describe('readableRange', () => {
    const cases = [
      ['now-1m', 'now', 'Last 1 minute'],
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
      [1624278889, 1640090089, '2021-06-21 12:34 PM - 2021-12-21 12:34 PM'],
    ];

    test.each(cases)(
      'readableRange(%s, %s) should be %s',
      (from, until, expected) => {
        expect(readableRange(from, until)).toBe(expected);
      }
    );
  });

  describe('formatAsOBject', () => {
    afterEach(() => {
      jest.restoreAllMocks();
    });

    it('works with "now"', () => {
      const mockDate = new Date();
      jest
        .spyOn(global, 'Date')
        .mockImplementation(() => mockDate as unknown as string);

      expect(formatAsOBject('now')).toBe(mockDate);
    });

    it('works with "now-1m"', () => {
      const mockDate = new Date('2021-12-21T12:44:01.741Z');

      jest
        .spyOn(global, 'Date')
        .mockImplementation(() => mockDate as unknown as string);

      expect(formatAsOBject('now-1m')).toBe(1640090641741);
    });

    it('works with absolute timestamps', () => {
      expect(formatAsOBject('1624192489')).toEqual(
        new Date('2021-06-20T12:34:49.000Z')
      );
    });
  });
});
