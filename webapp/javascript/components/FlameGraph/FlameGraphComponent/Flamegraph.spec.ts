import { Units } from '@utils/format';
import Flamegraph from './Flamegraph';
import { BAR_HEIGHT } from './constants';
import TestData from './testData';

jest.mock('./Flamegraph_render');

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

  describe('xyToBarData', () => {
    describe('single', () => {
      beforeEach(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom = { i: -1, j: -1 };
        const focusedNode = { i: -1, j: -1 };

        flame = new Flamegraph(
          flamebearerSingle,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom
        );
      });

      it('maps total row correctly', () => {
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
        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom = { i: 2, j: 8 };
        const focusedNode = { i: -1, j: -1 };

        flame = new Flamegraph(
          flamebearerSingle,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom
        );

        // now that same item should be available on x=0
        expect(flame.xyToBarData(1, BAR_HEIGHT * 3)).toMatchObject({
          format: 'single',
          name: 'main.slowFunction',
          offset: 217,
          self: 0,
          total: 771,
        });
      });

      //      it.only('maps even when focused on a node', () => {
      //        // canvas = document.createElement('canvas');
      //        // canvas.width = CANVAS_WIDTH;
      //        // canvas.height = CANVAS_HEIGHT;
      //
      //        const fitMode = 'HEAD';
      //        const highlightQuery = '';
      //        const zoom = { i: -1, j: -1 };
      //
      //        // main.fastFunction
      //        const focusedNode = { i: 2, j: 0 };
      //
      //        flame = new Flamegraph(
      //          TestData.SimpleTree,
      //          canvas,
      //          focusedNode,
      //          fitMode,
      //          highlightQuery,
      //          zoom
      //        );
      //
      //        //        expect(flame.xyToBarData(1, 0)).toMatchObject({
      //        //          format: 'single',
      //        //          name: 'total',
      //        //          offset: 217,
      //        //          self: 0,
      //        //          total: 771,
      //        //        });
      //
      //        expect(flame.xyToBarData(1, BAR_HEIGHT + 1)).toMatchObject({
      //          format: 'single',
      //          name: 'main.fastFunction',
      //          offset: 217,
      //          self: 0,
      //          total: 771,
      //        });
      //      });
    });

    describe('double', () => {
      beforeAll(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom = { i: -1, j: -1 };
        const focusedNode = { i: -1, j: -1 };

        flame = new Flamegraph(
          flamebearerDouble,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom
        );
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
  });

  describe('isWithinBounds', () => {
    beforeEach(() => {
      canvas = document.createElement('canvas');
      canvas.width = CANVAS_WIDTH;
      canvas.height = CANVAS_HEIGHT;

      const fitMode = 'HEAD';
      const highlightQuery = '';
      const focusedNode = { i: -1, j: -1 };
      const zoom = { i: 2, j: 8 };

      flame = new Flamegraph(
        flamebearerSingle,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
      );

      flame.render();
    });
    it('handles within canvas', () => {
      expect(flame.isWithinBounds(0, 0)).toBe(true);
      expect(flame.isWithinBounds(CANVAS_WIDTH - 1, 0)).toBe(true);
      expect(flame.isWithinBounds(-1, 0)).toBe(false);
      expect(flame.isWithinBounds(0, -1)).toBe(false);
      expect(flame.isWithinBounds(-1, -1)).toBe(false);
    });
  });

  describe('xyToBarPosition', () => {
    beforeEach(() => {
      canvas = document.createElement('canvas');
      canvas.width = CANVAS_WIDTH;
      canvas.height = CANVAS_HEIGHT;

      const fitMode = 'HEAD';
      const highlightQuery = '';
      const zoom = { i: -1, j: -1 };
      const focusedNode = { i: -1, j: -1 };

      flame = new Flamegraph(
        flamebearerSingle,
        canvas,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
      );

      flame.render();
    });

    it('works with the first bar (total)', () => {
      const got = flame.xyToBarPosition(0, 0);
      expect(got).toMatchObject({
        x: 0,
        y: 0,
      });
    });

    it('works a full bar', () => {
      // 2nd line,
      const got = flame.xyToBarPosition(0, BAR_HEIGHT + 1);
      expect(got).toMatchObject({
        x: 0,
        y: 22,
      });
      expect(got.width).toBeCloseTo(CANVAS_WIDTH);
    });

    it('works with a non full bar', () => {
      // 3nd line, 'slowFunction'
      const got = flame.xyToBarPosition(1, BAR_HEIGHT * 3);

      expect(got).toMatchObject({
        x: 0,
        y: 44,
      });
      expect(got.width).toBeCloseTo(129.95951417004048);
    });

    //        expect(flame.xyToBarData(1, BAR_HEIGHT + 1)).toMatchObject({
    //          format: 'single',
    //          name: 'main.fastFunction',
    //          offset: 217,
    //          self: 0,
    //          total: 771,
    //        });
  });
  describe('xyToBar', () => {
    beforeAll(() => {
      canvas = document.createElement('canvas');
      canvas.width = CANVAS_WIDTH;
      canvas.height = CANVAS_HEIGHT;
    });

    describe('focused', () => {
      beforeAll(() => {
        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom = { i: -1, j: -1 };

        // main.main
        const focusedNode = { i: 1, j: 0 };

        flame = new Flamegraph(
          TestData.SimpleTree,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom
        );
      });

      it('matches the total bar', () => {
        expect(flame.xyToBar(1, 0)).toMatchObject({ i: 0, j: 0 });
      });

      it('matches the focused node on the first row', () => {
        expect(flame.xyToBar(1, BAR_HEIGHT + 1)).toMatchObject({
          i: 1,
          j: 0,
        });
      });

      it('matches the focused node on the second row', () => {
        expect(flame.xyToBar(1, BAR_HEIGHT * 2 + 1)).toMatchObject({
          i: 2,
          j: 0,
        });
      });
    });

    // TODO (only zoomed)

    describe('focused and zoomed', () => {
      beforeAll(() => {
        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom = { i: 2, j: 0 };

        // main.main
        const focusedNode = { i: 1, j: 0 };

        flame = new Flamegraph(
          TestData.SimpleTree,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom
        );
      });

      it('matches the total bar', () => {
        expect(flame.xyToBar(1, 0)).toMatchObject({ i: 0, j: 0 });
      });

      it('matches the focused node on the first row', () => {
        expect(flame.xyToBar(1, BAR_HEIGHT + 1)).toMatchObject({
          i: 1,
          j: 0,
        });
      });

      it('matches a node on the left on the second row', () => {
        expect(flame.xyToBar(1, BAR_HEIGHT * 2 + 1)).toMatchObject({
          i: 2,
          j: 0,
        });
      });

      // that same node should've been expanded to full width
      it('matches a node on the right on the second row', () => {
        expect(
          flame.xyToBar(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1)
        ).toMatchObject({
          i: 2,
          j: 0,
        });
      });
    });
  });

  describe.only('xyToBarPosition 2', () => {
    describe('normal', () => {
      beforeAll(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        const fitMode = 'HEAD';
        const highlightQuery = '';
        const zoom = { i: -1, j: -1 };
        const focusedNode = { i: -1, j: -1 };

        flame = new Flamegraph(
          TestData.SimpleTree,
          canvas,
          focusedNode,
          fitMode,
          highlightQuery,
          zoom
        );

        flame.render();
      });

      it('works with the first bar (total)', () => {
        const got = flame.xyToBar2(0, 0);
        expect(got.x).toBe(0);
        expect(got.y).toBe(0);
        expect(got.width).toBeCloseTo(CANVAS_WIDTH);
      });

      it('works a full bar (runtime.main)', () => {
        // 2nd line,
        const got = flame.xyToBar2(0, BAR_HEIGHT + 1);
        expect(got.x).toBe(0);
        expect(got.y).toBe(22);
        expect(got.width).toBeCloseTo(CANVAS_WIDTH);
      });

      it('works with (main.fastFunction)', () => {
        // 3nd line, 'slowFunction'
        const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);

        expect(got.x).toBe(0);
        expect(got.y).toBe(44);
        expect(got.width).toBeCloseTo(129.95951417004048);
      });

      it('works with (main.slowFunction)', () => {
        // 3nd line, 'slowFunction'
        const got = flame.xyToBar2(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1);

        expect(got.x).toBeCloseTo(131.78);
        expect(got.y).toBe(44);
        expect(got.width).toBeCloseTo(468.218);
      });

      describe('boundary testing', () => {
        const cases = [
          [0, 0],
          [CANVAS_WIDTH, 0],
          [1, BAR_HEIGHT],
          [CANVAS_WIDTH, BAR_HEIGHT],
          [CANVAS_WIDTH / 2, BAR_HEIGHT / 2],
        ];
        test.each(cases)(
          'given %p and %p as arguments, returns the total bar',
          (i: number, j: number) => {
            const got = flame.xyToBar2(i, j);
            expect(got).toMatchObject({
              i: 0,
              j: 0,
              x: 0,
              y: 0,
            });

            expect(got.width).toBeCloseTo(CANVAS_WIDTH);
          }
        );
      });
    });

    describe('focused', () => {
      describe('on the first row (runtime.main)', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom = { i: -1, j: -1 };
          const focusedNode = { i: 1, j: 0 };

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar2(0, 0);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame.xyToBar2(0, BAR_HEIGHT + 1);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        //
        it('works with (main.fastFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);

          expect(got).toMatchObject({
            i: 2,
            j: 0,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(129.95951417004048);
        });
        //
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame.xyToBar2(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
          });
          expect(got.x).toBeCloseTo(131.78);
          expect(got.y).toBe(44);
          expect(got.width).toBeCloseTo(468.218);
        });
      });

      describe('on main.slowFunction', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom = { i: -1, j: -1 };
          const focusedNode = { i: 2, j: 8 };

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom
          );

          flame.render();
        });

        it('works with the first row (total)', () => {
          const got = flame.xyToBar2(0, 0);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with itself as second row (main.slowFunction)', () => {
          // 2nd line,
          const got = flame.xyToBar2(1, BAR_HEIGHT + 1);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with its child as third row (main.work)', () => {
          // 2nd line,
          const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);

          expect(got).toMatchObject({
            i: 3,
            j: 8,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //

        //        it('works with (main.fastFunction)', () => {
        //          // 3nd line, 'slowFunction'
        //          const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);
        //
        //          expect(got).toMatchObject({
        //            i: 2,
        //            j: 8,
        //            x: 0,
        //            y: 44,
        //          });
        //
        //          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        //        });
        //
        //        it('works with (main.slowFunction)', () => {
        //          // 3nd line, 'slowFunction'
        //          const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);
        //
        //          expect(got).toMatchObject({
        //            i: 2,
        //            j: 8,
        //            x: 0,
        //            y: 44,
        //          });
        //
        //          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        //        });
        //  it('works with main.work (child of main.slowFunction)', () => {
        //    // 3nd line, 'slowFunction'
        //    const got = flame.xyToBar2(1, BAR_HEIGHT * 3 + 1);

        //    expect(got).toMatchObject({
        //      i: 3,
        //      j: 8,
        //      x: 0,
        //      y: 66,
        //    });
        //    expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        //  });
      });
    });

    describe('zoomed', () => {
      describe('on the first row (runtime.main)', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom = { i: 1, j: 0 };
          const focusedNode = { i: -1, j: -1 };

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar2(0, 0);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame.xyToBar2(0, BAR_HEIGHT + 1);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        //
        it('works with (main.fastFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);

          expect(got).toMatchObject({
            i: 2,
            j: 0,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(129.95951417004048);
        });
        //
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame.xyToBar2(CANVAS_WIDTH - 1, BAR_HEIGHT * 2 + 1);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
          });
          expect(got.x).toBeCloseTo(131.78);
          expect(got.y).toBe(44);
          expect(got.width).toBeCloseTo(468.218);
        });
      });

      describe('on main.slowFunction', () => {
        beforeAll(() => {
          canvas = document.createElement('canvas');
          canvas.width = CANVAS_WIDTH;
          canvas.height = CANVAS_HEIGHT;

          const fitMode = 'HEAD';
          const highlightQuery = '';
          const zoom = { i: 2, j: 8 };
          const focusedNode = { i: -1, j: -1 };

          flame = new Flamegraph(
            TestData.SimpleTree,
            canvas,
            focusedNode,
            fitMode,
            highlightQuery,
            zoom
          );

          flame.render();
        });

        it('works with the first bar (total)', () => {
          const got = flame.xyToBar2(0, 0);
          expect(got.x).toBe(0);
          expect(got.y).toBe(0);
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        it('works with a full bar (runtime.main)', () => {
          // 2nd line,
          const got = flame.xyToBar2(0, BAR_HEIGHT + 1);

          expect(got).toMatchObject({
            i: 1,
            j: 0,
            x: 0,
            y: 22,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
        //
        //
        it('works with (main.slowFunction)', () => {
          // 3nd line, 'slowFunction'
          const got = flame.xyToBar2(1, BAR_HEIGHT * 2 + 1);

          expect(got).toMatchObject({
            i: 2,
            j: 8,
            x: 0,
            y: 44,
          });

          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });

        it('works with main.work (child of main.slowFunction)', () => {
          // 4th line, 'main.work'
          // TODO why 2??
          const got = flame.xyToBar2(1, BAR_HEIGHT * 3 + 2);

          expect(got).toMatchObject({
            i: 3,
            j: 8,
            x: 0,
            y: 66,
          });
          expect(got.width).toBeCloseTo(CANVAS_WIDTH);
        });
      });
    });
  });
});
