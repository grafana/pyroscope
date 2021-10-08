import { numberWithCommas } from './format';

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
});
