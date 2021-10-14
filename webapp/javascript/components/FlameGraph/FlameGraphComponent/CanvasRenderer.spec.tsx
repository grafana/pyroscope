/* eslint-disable no-underscore-dangle */
import CanvasConverter from 'canvas-to-buffer';
import { createCanvas } from 'canvas';
import { Units } from '../../../util/format';
import { RenderCanvas } from './CanvasRenderer';

const { toMatchImageSnapshot } = require('jest-image-snapshot');

describe('CanvasRenderer', () => {
  it('works with normal flamegraph', () => {
    const canvas = createCanvas(800, 600) as unknown as HTMLCanvasElement;

    RenderCanvas({
      canvas,
      // necessary, otherwise `clientWidth` is undefined
      canvasWidth: 800,
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

      rangeMax: 1,
      units: Units.Samples,
      fitMode: 'HEAD',

      font: 'monospace',
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('works with highlighted flamegraph', () => {
    const canvas = createCanvas(800, 600) as unknown as HTMLCanvasElement;

    RenderCanvas({
      canvas,
      // necessary, otherwise `clientWidth` is undefined
      canvasWidth: 800,
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

      highlightQuery: 'main.work',
      rangeMax: 1,
      units: Units.Samples,
      fitMode: 'HEAD',

      font: 'monospace',
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });
});

// since this is the only test that uses snapshot testing
expect.extend({ toMatchImageSnapshot });
function canvasToBuffer(canvas: HTMLCanvasElement) {
  const converter = new CanvasConverter(canvas, {
    image: { types: ['png'] },
  });

  return converter.toBuffer();
}
