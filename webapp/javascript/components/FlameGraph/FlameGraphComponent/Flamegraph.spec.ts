import { Units } from '@utils/format';
import Flamegraph from './Flamegraph';
import { RenderCanvas } from './CanvasRenderer';
import { BAR_HEIGHT } from './constants';
import TestData from './testData';

jest.mock('./CanvasRenderer');

const format: 'single' | 'double' = 'single';
const flamebearerSingle = {
  format,
  numTicks: 988,
  sampleRate: 100,
  names: [
    'total',
    'runtime.main',
    'main.slowFunction',
    'main.work',
    'main.main',
    'main.fastFunction',
  ],
  levels: [
    [0, 988, 0, 0],
    [0, 988, 0, 1],
    [0, 214, 0, 5, 214, 3, 2, 4, 217, 771, 0, 2],
    [0, 214, 214, 3, 216, 1, 1, 5, 217, 771, 771, 3],
  ],
  units: Units.Samples,
  spyName: 'gospy',
};

const flamebearerDouble = {
  names: [
    'total',
    'runtime/pprof.profileWriter',
    'runtime.mcall',
    'runtime.park_m',
    'runtime.schedule',
    'runtime.findrunnable',
    'runtime.netpoll',
    'runtime.epollwait',
    'runtime.main',
    'main.slowFunction',
    'main.work',
    'fmt.Printf',
    'fmt.Fprintf',
    'os.(*File).write',
    'syscall.Write',
    'syscall.write',
    'syscall.Syscall',
    'runtime.exitsyscall',
    'runtime.exitsyscallfast',
    'runtime.wirep',
    'main.fastFunction',
  ],
  levels: [
    [0, 246, 0, 0, 986, 0, 0],
    [0, 245, 0, 0, 985, 0, 8, 245, 1, 0, 985, 0, 0, 2, 246, 0, 0, 985, 1, 1, 1],
    [
      0, 49, 0, 0, 181, 0, 20, 49, 196, 0, 181, 804, 0, 9, 245, 1, 0, 985, 0, 0,
      3,
    ],
    [
      0, 49, 49, 0, 181, 181, 10, 49, 0, 0, 181, 1, 0, 11, 49, 196, 196, 182,
      803, 803, 10, 245, 1, 0, 985, 0, 0, 4,
    ],
    [49, 0, 0, 181, 1, 0, 12, 245, 1, 0, 985, 0, 0, 5],
    [49, 0, 0, 181, 1, 0, 13, 245, 1, 0, 985, 0, 0, 6],
    [49, 0, 0, 181, 1, 0, 14, 245, 1, 1, 985, 0, 0, 7],
    [49, 0, 0, 181, 1, 0, 15],
    [49, 0, 0, 181, 1, 0, 16],
    [49, 0, 0, 181, 1, 0, 17],
    [49, 0, 0, 181, 1, 0, 18],
    [49, 0, 0, 181, 1, 1, 19],
  ],
  numTicks: 1232,
  maxSelf: 803,
  spyName: 'gospy',
  sampleRate: 100,
  units: Units.Samples,
  format: 'double' as const,
  leftTicks: 246,
  rightTicks: 986,
};

describe('Flamegraph', () => {
  let canvas: any;
  let flame: Flamegraph;
  const CANVAS_WIDTH = 600;
  const CANVAS_HEIGHT = 300;

  it('renders canvas using RenderCanvas', () => {
    canvas = document.createElement('canvas');
    canvas.width = CANVAS_WIDTH;
    canvas.height = CANVAS_HEIGHT;

    flame = new Flamegraph(flamebearerSingle, canvas, 'HEAD');

    flame.render();
    expect(RenderCanvas).toHaveBeenCalled();
  });

  describe('xyToBarData', () => {
    describe('single', () => {
      beforeEach(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        flame = new Flamegraph(flamebearerSingle, canvas, 'HEAD');
      });

      it('maps total correctly', () => {
        expect(flame.xyToBarData(0, 0)).toStrictEqual({
          format: 'single',
          name: 'total',
          offset: 0,
          self: 0,
          total: 988,
        });
      });

      it('maps a full row correctly', () => {
        expect(flame.xyToBarData(1, BAR_HEIGHT + 1)).toStrictEqual({
          format: 'single',
          name: 'runtime.main',
          offset: 0,
          self: 0,
          total: 988,
        });
      });

      it('maps a row with more items', () => {
        expect(flame.xyToBarData(1, BAR_HEIGHT * 2 + 1)).toStrictEqual({
          format: 'single',
          name: 'main.fastFunction',
          offset: 0,
          self: 0,
          total: 214,
        });

        expect(
          flame.xyToBarData(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1)
        ).toStrictEqual({
          format: 'single',
          name: 'main.slowFunction',
          offset: 217,
          self: 0,
          total: 771,
        });
      });

      it('maps correctly even when zoomed in', () => {
        // third row, last item (main.slowFunction)
        expect(flame.xyToBarData(canvas.width, BAR_HEIGHT * 3)).toStrictEqual({
          format: 'single',
          name: 'main.slowFunction',
          offset: 217,
          self: 0,
          total: 771,
        });

        // there's a different item under x=0
        expect(flame.xyToBarData(1, BAR_HEIGHT * 3)).not.toMatchObject({
          format: 'single',
          name: 'main.slowFunction',
          offset: 217,
          self: 0,
          total: 771,
        });

        // zoom on that item
        flame.zoom(2, 8);

        // now that same item should be available on x=0
        expect(flame.xyToBarData(1, BAR_HEIGHT * 3)).toMatchObject({
          format: 'single',
          name: 'main.slowFunction',
          offset: 217,
          self: 0,
          total: 771,
        });
      });
    });

    describe('double', () => {
      beforeAll(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        flame = new Flamegraph(flamebearerDouble, canvas, 'HEAD');
      });

      it('maps total correctly', () => {
        expect(flame.xyToBarData(0, 0)).toStrictEqual({
          format: 'double',
          name: 'total',
          totalLeft: 246,
          totalRight: 986,
          barTotal: 1232,
          totalDiff: 740,
        });
      });

      it('maps a full row correctly', () => {
        expect(flame.xyToBarData(1, BAR_HEIGHT + 1)).toStrictEqual({
          format: 'double',
          name: 'runtime.main',
          totalLeft: 245,
          totalRight: 985,
          barTotal: 1230,
          totalDiff: 740,
        });
      });

      it('maps a row with more items', () => {
        expect(flame.xyToBarData(1, BAR_HEIGHT * 2 + 1)).toStrictEqual({
          format: 'double',
          name: 'main.fastFunction',
          totalLeft: 49,
          totalRight: 181,
          barTotal: 230,
          totalDiff: 132,
        });

        expect(
          flame.xyToBarData(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1)
        ).toStrictEqual({
          format: 'double',
          name: 'main.slowFunction',
          totalDiff: 608,
          totalLeft: 196,
          totalRight: 804,
          barTotal: 1000,
        });
      });

      // TODO:
      // test when it's zoomed?
    });

    // TODO tests for focused item
    // TODO tests for double
  });

  describe('isWithinBounds', () => {
    beforeEach(() => {
      canvas = document.createElement('canvas');
      canvas.width = CANVAS_WIDTH;
      canvas.height = CANVAS_HEIGHT;

      flame = new Flamegraph(flamebearerSingle, canvas, 'HEAD');
    });
    it('handles within canvas', () => {
      expect(flame.isWithinBounds(0, 0)).toBe(true);
      expect(flame.isWithinBounds(CANVAS_WIDTH, 0)).toBe(true);
      expect(flame.isWithinBounds(-1, 0)).toBe(false);
      expect(flame.isWithinBounds(0, -1)).toBe(false);
      expect(flame.isWithinBounds(-1, -1)).toBe(false);
    });

    //    it('returns false when is within canvas but outside a bar', () => {
    //      // TODO: this shouldn have worked...
    //      expect(flame.isWithinBounds(CANVAS_WIDTH, CANVAS_HEIGHT)).toBe(false);
    //    });
  });

  describe('xyToBarPosition', () => {
    beforeEach(() => {
      canvas = document.createElement('canvas');
      canvas.width = CANVAS_WIDTH;
      canvas.height = CANVAS_HEIGHT;

      flame = new Flamegraph(flamebearerSingle, canvas, 'HEAD');
    });

    it('works with the first bar (total)', () => {
      expect(flame.xyToBarPosition(0, 0)).toMatchObject({
        x: 0,
        y: 0,
        width: CANVAS_WIDTH,
      });
    });

    it('works a full bar', () => {
      // 2nd line,
      expect(flame.xyToBarPosition(0, BAR_HEIGHT + 1)).toMatchObject({
        x: 0,
        y: 22,
        width: CANVAS_WIDTH,
      });
    });

    it('works with a non full bar', () => {
      // 3nd line, 'slowFunction'
      expect(flame.xyToBarPosition(0, BAR_HEIGHT * 2 + 1)).toMatchObject({
        x: 0,
        y: 44,
        width: 129.95951417004048,
      });
    });
  });

  //  describe('xyToTooltippData', () => {
  //    describe('single', () => {
  //      it('works with total row', () => {
  //        expect(flame.xyToTooltipData('single', 0, 0)).toMatchObject({
  //          format: 'single',
  //          title: 'total',
  //          numBarTicks: 988,
  //          percent: '100%',
  //        });
  //      });
  //
  //      it('works with full row', () => {
  //        expect(
  //          flame.xyToTooltipData('single', 0, BAR_HEIGHT + 1)
  //        ).toMatchObject({
  //          format: 'single',
  //          title: 'runtime.main',
  //          numBarTicks: 988,
  //          percent: '100%',
  //        });
  //      });
  //
  //      it('works with divided row', () => {
  //        expect(
  //          flame.xyToTooltipData('single', CANVAS_WIDTH, BAR_HEIGHT * 3)
  //        ).toMatchObject({
  //          format: 'single',
  //          title: 'main.slowFunction',
  //          numBarTicks: 771,
  //          percent: '78.04%',
  //        });
  //      });
  //
  //      it('throws an error if format is incompatible', () => {
  //        expect(() => flame.xyToTooltipData('double', 0, 0)).toThrow();
  //      });
  //    });
  //
  //    describe('double', () => {
  //      beforeEach(() => {
  //        canvas = document.createElement('canvas');
  //        canvas.width = CANVAS_WIDTH;
  //        canvas.height = CANVAS_HEIGHT;
  //
  //        flame = new Flamegraph(
  //          { ...flamebearerDouble, format: 'double' },
  //          canvas,
  //          'HEAD'
  //        );
  //      });
  //
  //      it('works with full row', () => {
  //        expect(
  //          flame.xyToTooltipData('double', 1, BAR_HEIGHT + 1)
  //        ).toMatchObject({
  //          format: 'double',
  //          title: 'runtime.main',
  //          left: 991,
  //          right: 985,
  //          leftPercent: 100,
  //          rightPercent: 100,
  //        });
  //      });
  //
  //      it('works with divided row', () => {
  //        expect(
  //          flame.xyToTooltipData('double', CANVAS_WIDTH - 2, BAR_HEIGHT * 2 + 1)
  //        ).toMatchObject({
  //          format: 'double',
  //          title: 'runtime.main',
  //          left: 991,
  //          right: 985,
  //          leftPercent: 100,
  //          rightPercent: 100,
  //        });
  //      });
  //      // TODO
  //      //        expect(() => flame.xyToTooltipData('single', 0, 0)).toThrow();
  //      //      });
  //    });
  //  });
});
