/* eslint-disable no-underscore-dangle */
import CanvasConverter from 'canvas-to-buffer';
import { createCanvas } from 'canvas';
import { Units } from '../../../util/format';
import { RenderCanvas } from './CanvasRenderer';
// since this is the only test that uses snapshot testing
const { toMatchImageSnapshot } = require('jest-image-snapshot');

expect.extend({ toMatchImageSnapshot });

function canvasToBuffer(canvas: HTMLCanvasElement) {
  const converter = new CanvasConverter(canvas, {
    image: { types: ['png'] },
  });

  return converter.toBuffer();
}

describe('CanvasRenderer', () => {
  let canvas: HTMLCanvasElement;
  let ctx: CanvasRenderingContext2D;
  beforeEach(() => {
    // canvas = document.createElement('canvas');
    canvas = createCanvas(400, 400) as unknown as HTMLCanvasElement;
    // canvas.width = 400;
    // canvas.height = 400;
    ctx = canvas.getContext('2d');
  });

  it('works', () => {
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
    });

    //    const context = canvas.getContext('2d');
    //
    //    context.beginPath();
    //    context.lineWidth = 6;
    //    context.fillStyle = 'red';
    //    context.strokeStyle = 'red';
    //    context.rect(0, 0, 800, 600);
    //    context.stroke();
    //    context.fill();
    //
    //    expect(ctx.__getEvents()).toMatchSnapshot();
    //    expect(canvas.toDataURL('image/png')).toMatchImageSnapshot();
    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });
});
