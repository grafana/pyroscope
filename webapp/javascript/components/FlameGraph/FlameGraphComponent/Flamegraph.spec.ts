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
    'runtime.main',
    'main.main',
    'main.becomesAdded',
    'main.becomesSlower',
    'main.work',
    'runtime.asyncPreempt',
    'main.becomesFaster',
    'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote.(*Remote).handleJobs',
    'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote.(*Remote).safeUpload',
    'net/http.(*Client).Do',
    'net/http.(*Client).do',
    'net/http.(*Client).send',
    'net/http.send',
    'net/http.(*Transport).RoundTrip',
    'net/http.setupRewindBody',
    'runtime.newobject',
    'runtime.mallocgc',
    'runtime.arenaIndex',
    'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots',
    'runtime.heapBitsSetType',
  ],
  levels: [
    [0, 991, 0, 0, 987, 0, 0],
    [0, 0, 0, 0, 1, 0, 19, 0, 0, 0, 1, 1, 0, 8, 0, 991, 0, 2, 985, 0, 1],
    [
      0, 0, 0, 0, 1, 0, 16, 0, 0, 0, 1, 1, 0, 9, 0, 217, 0, 2, 229, 0, 3, 217,
      165, 0, 231, 147, 0, 7, 382, 603, 1, 378, 604, 0, 4, 985, 6, 6, 982, 5, 4,
      2,
    ],
    [
      0, 0, 0, 0, 1, 0, 17, 0, 0, 0, 1, 1, 0, 10, 0, 217, 217, 2, 229, 229, 5,
      217, 165, 165, 231, 147, 147, 5, 383, 602, 601, 378, 604, 604, 5, 991, 0,
      0, 986, 1, 1, 3,
    ],
    [0, 0, 0, 0, 1, 1, 20, 0, 0, 0, 1, 1, 0, 11, 984, 1, 1, 982, 0, 0, 6],
    [0, 0, 0, 1, 1, 0, 12],
    [0, 0, 0, 1, 1, 0, 13],
    [0, 0, 0, 1, 1, 0, 14],
    [0, 0, 0, 1, 1, 0, 15],
    [0, 0, 0, 1, 1, 0, 16],
    [0, 0, 0, 1, 1, 0, 17],
    [0, 0, 0, 1, 1, 1, 18],
  ],
  numTicks: 1978,
  maxSelf: 604,
  spyName: 'gospy',
  sampleRate: 100,
  units: Units.Samples,
  format: 'double' as const,
  leftTicks: 991,
  rightTicks: 987,
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

  describe('xyToBar', () => {
    describe('single', () => {
      beforeEach(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        flame = new Flamegraph(flamebearerSingle, canvas, 'HEAD');
      });

      it('maps correcly', () => {
        expect(flame.xyToBar(0, 0)).toMatchObject({ i: 0, j: 0 });

        // second row
        expect(flame.xyToBar(0, BAR_HEIGHT * 2)).toMatchObject({ i: 1, j: 0 });

        // third row
        expect(flame.xyToBar(0, BAR_HEIGHT * 3)).toMatchObject({ i: 2, j: 0 });

        // third row, last item
        expect(flame.xyToBar(canvas.width, BAR_HEIGHT * 3)).toMatchObject({
          i: 2,
          j: 8,
        });
      });

      it('maps correctly even when zoomed in', () => {
        // third row, last item (main.slowFunction)
        expect(flame.xyToBar(canvas.width, BAR_HEIGHT * 3)).toMatchObject({
          i: 2,
          j: 8,
        });
        // zoom on that item
        flame.zoom(2, 8);

        // now that same item should be available under 0,0
        // the 20px there is due to the calculations being messed up when it's right on the border
        expect(flame.xyToBar(0 + 20, BAR_HEIGHT * 3)).toMatchObject({
          i: 2,
          j: 8,
        });
      });
    });

    describe.only('double', () => {
      beforeEach(() => {
        canvas = document.createElement('canvas');
        canvas.width = CANVAS_WIDTH;
        canvas.height = CANVAS_HEIGHT;

        flame = new Flamegraph(flamebearerDouble, canvas, 'HEAD');
      });

      it('maps total correctly', () => {
        expect(flame.xyToBar(0, 0)).toMatchObject({ i: 0, j: 0 });
      });

      // TODO
      // figure out this
      it('maps second row correctly', () => {
        expect(flame.xyToBar(0, BAR_HEIGHT * 2)).toMatchObject({ i: 1, j: 0 });
      });
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
  //    describe.only('double', () => {
  //      beforeEach(() => {
  //        //        flame = new Flamegraph({ ...data, format: 'double' }, canvas, 'HEAD');
  //      });
  //
  //      it('works with full row', () => {
  //        const data = {
  //          names: [
  //            'total',
  //            'runtime.main',
  //            'main.main',
  //            'main.becomesAdded',
  //            'main.becomesSlower',
  //            'main.work',
  //            'runtime.asyncPreempt',
  //            'main.becomesFaster',
  //            'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote.(*Remote).handleJobs',
  //            'github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote.(*Remote).safeUpload',
  //            'net/http.(*Client).Do',
  //            'net/http.(*Client).do',
  //            'net/http.(*Client).send',
  //            'net/http.send',
  //            'net/http.(*Transport).RoundTrip',
  //            'net/http.setupRewindBody',
  //            'runtime.newobject',
  //            'runtime.mallocgc',
  //            'runtime.arenaIndex',
  //            'github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots',
  //            'runtime.heapBitsSetType',
  //          ],
  //          levels: [
  //            [0, 991, 0, 0, 987, 0, 0],
  //            [
  //              0, 0, 0, 0, 1, 0, 19, 0, 0, 0, 1, 1, 0, 8, 0, 991, 0, 2, 985, 0,
  //              1,
  //            ],
  //            [
  //              0, 0, 0, 0, 1, 0, 16, 0, 0, 0, 1, 1, 0, 9, 0, 217, 0, 2, 229, 0,
  //              3, 217, 165, 0, 231, 147, 0, 7, 382, 603, 1, 378, 604, 0, 4, 985,
  //              6, 6, 982, 5, 4, 2,
  //            ],
  //            [
  //              0, 0, 0, 0, 1, 0, 17, 0, 0, 0, 1, 1, 0, 10, 0, 217, 217, 2, 229,
  //              229, 5, 217, 165, 165, 231, 147, 147, 5, 383, 602, 601, 378, 604,
  //              604, 5, 991, 0, 0, 986, 1, 1, 3,
  //            ],
  //            [
  //              0, 0, 0, 0, 1, 1, 20, 0, 0, 0, 1, 1, 0, 11, 984, 1, 1, 982, 0, 0,
  //              6,
  //            ],
  //            [0, 0, 0, 1, 1, 0, 12],
  //            [0, 0, 0, 1, 1, 0, 13],
  //            [0, 0, 0, 1, 1, 0, 14],
  //            [0, 0, 0, 1, 1, 0, 15],
  //            [0, 0, 0, 1, 1, 0, 16],
  //            [0, 0, 0, 1, 1, 0, 17],
  //            [0, 0, 0, 1, 1, 1, 18],
  //          ],
  //          numTicks: 1978,
  //          maxSelf: 604,
  //          spyName: 'gospy',
  //          sampleRate: 100,
  //          units: Units.Samples,
  //          format: 'double',
  //          leftTicks: 991,
  //          rightTicks: 987,
  //        };
  //        console.log('creating double');
  //        const flame2 = new Flamegraph(
  //          { ...data, format: 'double' },
  //          canvas,
  //          'HEAD'
  //        );
  //
  //        const { i, j } = flame2.xyToBar(0, BAR_HEIGHT * 2);
  //        console.log({ i, j });
  //        //        expect(flame2.barToTitle(i, j)).toBe(true);
  //
  //        //        expect(
  //        //          flame2.xyToTooltipData('double', 0, BAR_HEIGHT * 4)
  //        //        ).toMatchObject({
  //        //          format: 'double',
  //        //          title: 'bla',
  //        //          numBarTicks: 988,
  //        //          percent: '100%',
  //        //          left: '',
  //        //          right: '',
  //        //          leftPercent: '',
  //        //          rightPercent: '',
  //        //        });
  //      });
  //
  //      // TODO
  //      //        expect(() => flame.xyToTooltipData('single', 0, 0)).toThrow();
  //      //      });
  //    });
  //  });
});
