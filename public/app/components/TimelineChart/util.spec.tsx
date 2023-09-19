import { TimelineVisibleData, rangeIsAcceptableForZoom } from './util';

describe('TimelineChart.util', function () {
  describe('rangeIsAcceptableForZoom', function () {
    const xaxisRange = { from: 10, to: 20 };
    function makeData(timeValues: number[]) {
      // The y value of 1 is used, but not actually read
      return {
        data: timeValues.map((time) => [time, 1]),
      } as TimelineVisibleData[number];
    }

    function executeOnData(data: TimelineVisibleData) {
      const pixels = 100000; // Should not factor in for this kind of test
      return rangeIsAcceptableForZoom(xaxisRange, data, pixels);
    }

    it('Rejects empty data', () => {
      expect(executeOnData([])).toBe(false);
      expect(executeOnData([makeData([]), makeData([])])).toBe(false);
    });

    it('Rejects if no points on xaxisrange', () => {
      expect(executeOnData([makeData([1, 2, 3, 4, 5, 6, 7, 8, 9])])).toBe(
        false
      );
    });

    it('Rejects only one point on xaxisrange', () => {
      expect(executeOnData([makeData([12])])).toBe(false);
      expect(executeOnData([makeData([2, 12, 22])])).toBe(false);
    });

    it('Rejects only one point on xaxisrange even when multiple datasets', () => {
      expect(
        executeOnData([makeData([12]), makeData([13]), makeData([14])])
      ).toBe(false);
      expect(
        executeOnData([
          makeData([2, 12, 22]),
          makeData([3, 13, 23]),
          makeData([4, 14, 24]),
        ])
      ).toBe(false);
    });

    it('Accepts if at least two points within xaxisrange', () => {
      expect(executeOnData([makeData([12, 13])])).toBe(true);
    });

    it('Accepts if at least one dataset has two points within xaxisrange', () => {
      expect(
        executeOnData([makeData([12]), makeData([13]), makeData([14, 15])])
      ).toBe(true);
    });

    it('Automatically accepts when more data than pixels', () => {
      const pixels = 10;
      const numPoints = 25;
      const data = [] as number[];
      for (let i = 0; i < numPoints; i++) {
        data.push(i);
      }
      expect(rangeIsAcceptableForZoom(xaxisRange, [makeData(data)], pixels));
    });
  });
});
