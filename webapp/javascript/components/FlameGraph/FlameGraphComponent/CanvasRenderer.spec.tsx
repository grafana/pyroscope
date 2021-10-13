/* eslint-disable no-underscore-dangle */
import { Units } from '../../../util/format';
import { RenderCanvas } from './CanvasRenderer';

describe('CanvasRenderer', () => {
  let canvas: HTMLCanvasElement;
  let ctx: CanvasRenderingContext2D;
  beforeEach(() => {
    canvas = document.createElement('canvas');
    canvas.width = 400;
    canvas.height = 400;
    ctx = canvas.getContext('2d');
  });

  it('works', () => {
    RenderCanvas({
      canvas,
      topLevel: 0,
      rangeMin: 0,
      pxPerTick: 0.43724696356275305,
      viewType: 'single',
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

      rangeMax: 0,
      units: Units.Samples,
      fitMode: 'HEAD',
    });

    expect(ctx.__getEvents()).toMatchSnapshot();
  });
});
