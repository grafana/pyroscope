import { calculateMean, calculateStdDeviation } from './math';

describe('math', () => {
  describe('calculateMean, calculateStdDeviation', () => {
    test.each([
      [[1, 2, 3, 4, 5], 1.4142135623730951],
      [[23, 4, 6, 457, 65, 7, 45, 8], 145.13565852332775],
      [[3456, 9876, 12, 0, 0, 99917, 1000000, 234543], 323657.1010328678],
    ])(
      'should calculate correct standart deviation',
      (array, expectedValue) => {
        const mean = calculateMean(array);

        expect(calculateStdDeviation(array, mean)).toBe(expectedValue);
      }
    );
  });
});
