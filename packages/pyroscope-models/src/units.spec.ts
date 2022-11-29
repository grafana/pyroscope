import { units, UnitsSchema } from './units';

describe('Units', function () {
  test.each(units.map((a) => [a, a]))(
    'parse("%s") should return "%s"',
    (spyName) => {
      expect(UnitsSchema.parse(spyName)).toBe(spyName);
    }
  );

  describe('when empty', () => {
    it('defaults to unknown', () => {
      expect(UnitsSchema.parse('')).toBe('unknown');
    });
  });
  describe('when a non-supported value is passed', () => {
    it('defaults to unknown', () => {
      expect(UnitsSchema.parse('UNKNOWN_UNIT')).toBe('unknown');
    });
  });
});
