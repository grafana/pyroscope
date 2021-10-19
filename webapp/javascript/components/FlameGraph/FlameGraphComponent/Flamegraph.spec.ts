import { Units } from '@utils/format';
import Flamegraph from './Flamegraph';
import { RenderCanvas } from './CanvasRenderer';
import { BAR_HEIGHT } from './constants';

jest.mock('./CanvasRenderer');

const viewType: 'single' | 'double' = 'single';
const flamebearer = {
  viewType,
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

  beforeEach(() => {
    canvas = document.createElement('canvas');
    canvas.width = 600;

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

    // TODO tests for focused item
  });
});
