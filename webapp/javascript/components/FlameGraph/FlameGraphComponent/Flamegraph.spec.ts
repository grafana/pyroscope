import { Units } from '@utils/format';
import Flamegraph from './Flamegraph';
import { RenderCanvas } from './CanvasRenderer';
import { BAR_HEIGHT } from './constants';

jest.mock('./CanvasRenderer');

const format: 'single' | 'double' = 'single';
const flamebearer = {
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

describe('Flamegraph', () => {
  let canvas: any;
  let flame: Flamegraph;
  const CANVAS_WIDTH = 600;
  const CANVAS_HEIGHT = 300;

  beforeEach(() => {
    canvas = document.createElement('canvas');
    canvas.width = CANVAS_WIDTH;
    canvas.height = CANVAS_HEIGHT;

    flame = new Flamegraph(flamebearer, canvas, 'HEAD');
  });

  it('renders canvas using RenderCanvas', () => {
    flame.render();
    expect(RenderCanvas).toHaveBeenCalled();
  });

  describe('xyToBar', () => {
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

    // TODO tests for focused item
  });

  describe('isWithinBounds', () => {
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

  describe.only('xyToBarPosition', () => {
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
});
