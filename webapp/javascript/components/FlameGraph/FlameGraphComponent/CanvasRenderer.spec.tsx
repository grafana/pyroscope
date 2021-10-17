/* eslint-disable no-underscore-dangle */
import CanvasConverter from 'canvas-to-buffer';
import { createCanvas } from 'canvas';
import { Units } from '../../../util/format';
import { RenderCanvas } from './CanvasRenderer';
import TestData from './testData';

const { toMatchImageSnapshot } = require('jest-image-snapshot');

describe('CanvasRenderer -- group:snapshot', () => {
  it('works with normal flamegraph', () => {
    const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;

    RenderCanvas({
      canvas,
      topLevel: 0,
      rangeMin: 0,
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

      //      font: 'monospace',
      spyName: 'gospy',
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('collapses small blocks into one', () => {
    const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;

    const data = TestData.ComplexTree;

    RenderCanvas({
      canvas,
      topLevel: 0,

      viewType: 'single',
      numTicks: data.numTicks,
      sampleRate: data.sampleRate,
      names: data.names,
      levels: data.levels,

      rangeMin: 0,
      rangeMax: 1,
      units: Units.Samples,
      fitMode: 'HEAD',

      spyName: data.spyName,
      //      font: 'monospace',
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('works with highlighted flamegraph', () => {
    const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;

    RenderCanvas({
      canvas,
      topLevel: 0,
      rangeMin: 0,
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
      spyName: 'gospy',

      //      font: 'monospace',
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it(`works with "selected" node`, () => {
    const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;

    RenderCanvas({
      canvas,
      //      topLevel: 0,
      rangeMin: 0,
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

      units: Units.Samples,
      fitMode: 'HEAD',
      spyName: 'gospy',

      //      font: 'monospace',

      selectedLevel: 2,
      topLevel: 0,

      // horrible api
      // TODO, receive the i/j ?
      rangeMax: 0.2165991902834008,
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });

  it('works with "diff" mode', () => {
    const canvas = createCanvas(800, 0) as unknown as HTMLCanvasElement;
    const data = TestData.DiffTree;

    RenderCanvas({
      canvas,
      topLevel: 0,

      viewType: 'double',
      numTicks: data.numTicks,
      sampleRate: data.sampleRate,
      names: data.names,
      levels: data.levels,

      rangeMin: 0,
      rangeMax: 1,
      units: Units.Samples,
      fitMode: 'HEAD',

      spyName: data.spyName,
      //      font: 'monospace',

      rightTicks: data.rightTicks,
      leftTicks: data.leftTicks,
    });

    expect(canvasToBuffer(canvas)).toMatchImageSnapshot();
  });
});

// since this is the only test that uses snapshot testing
// expect.extend({ toMatchImageSnapshot });
function canvasToBuffer(canvas: HTMLCanvasElement) {
  const converter = new CanvasConverter(canvas, {
    image: { types: ['png'] },
  });

  return converter.toBuffer();
}
