import { createFF } from '@utils/flamebearer';
import { Units } from '@utils/format';
import { RenderCanvas } from './CanvasRenderer';
import { PX_PER_LEVEL, BAR_HEIGHT, COLLAPSE_THRESHOLD } from './constants';

/* eslint-disable no-useless-constructor */

// if it's type double (diff), we also require `left` and `right` ticks
type addTicks =
  | { viewType: 'double'; leftTicks: number; rightTicks: number }
  | { viewType: 'single' };

type Flamebearer = {
  names: string[];
  levels: number[][];
  numTicks: number;
  sampleRate: number;
  units: Units;
  viewType: 'single' | 'double';
  spyName: string;
} & addTicks;

export default class Flamegraph {
  private ff: ReturnType<typeof createFF>;

  private topLevel: number;

  private rangeMin: number;

  private rangeMax: number;

  private selectedLevel: number;

  private pxPerWidth: number;

  private fitMode: 'HEAD' | 'TAIL';

  private canvas: HTMLCanvasElement;
  //  private readonly flamebearer: Flamebearer;

  constructor(
    private readonly flamebearer: Flamebearer,
    canvas: HTMLCanvasElement,
    fitMode: 'HEAD' | 'TAIL'
  ) {
    this.ff = createFF(flamebearer.viewType);

    this.fitMode = fitMode;

    this.topLevel = 0;
    this.rangeMin = 0;
    this.rangeMax = 1;

    this.selectedLevel = 0;

    this.canvas = canvas;
  }

  render() {
    const props = {
      canvas: this.canvas,

      viewType: this.flamebearer.viewType,
      numTicks: this.flamebearer.numTicks,
      sampleRate: this.flamebearer.sampleRate,
      names: this.flamebearer.names,
      levels: this.flamebearer.levels,
      spyName: this.flamebearer.spyName,
      units: this.flamebearer.units,

      topLevel: this.topLevel,
      rangeMin: this.rangeMin,
      rangeMax: this.rangeMax,
      fitMode: this.fitMode,
      selectedLevel: this.selectedLevel,
    };

    switch (this.flamebearer.viewType) {
      case 'single': {
        RenderCanvas({ ...props, viewType: 'single' });
        break;
      }
      case 'double': {
        RenderCanvas({
          ...props,
          leftTicks: this.flamebearer.leftTicks,
          rightTicks: this.flamebearer.rightTicks,
        });
        break;
      }
      default: {
        throw new Error(`Invalid format`);
      }
    }
  }

  private pxPerTick() {
    const graphWidth = this.canvas.width;

    return (
      graphWidth / this.flamebearer.numTicks / (this.rangeMax - this.rangeMin)
    );
  }

  private tickToX(i: number) {
    return (i - this.flamebearer.numTicks * this.rangeMin) * this.pxPerTick();
  }

  private getCanvasWidth() {
    // bit of a hack, but clientWidth is not available in node-canvas
    return this.canvas.clientWidth || this.canvas.width;
  }

  private isFocused() {
    return this.topLevel > 0;
  }

  // binary search of a block in a stack level
  // TODO(eh-am): calculations seem wrong when x is 0
  private binarySearchLevel(x: number, level: number[]) {
    const { ff } = this;

    let i = 0;
    let j = level.length - ff.jStep;
    while (i <= j) {
      /* eslint-disable-next-line no-bitwise */
      const m = ff.jStep * ((i / ff.jStep + j / ff.jStep) >> 1);
      const x0 = this.tickToX(ff.getBarOffset(level, m));
      const x1 = this.tickToX(
        ff.getBarOffset(level, m) + ff.getBarTotal(level, m)
      );

      if (x0 <= x && x1 >= x) {
        return x1 - x0 > COLLAPSE_THRESHOLD ? m : -1;
      }
      if (x0 > x) {
        j = m - ff.jStep;
      } else {
        i = m + ff.jStep;
      }
    }
    return -1;
  }

  xyToBar(x: number, y: number) {
    if (x < 0 || y < 0) {
      throw new Error(`x and y must be bigger than 0. x = ${x}, y = ${y}`);
    }

    // in focused mode there's a "fake" bar at the top
    // so we must discount for it
    const computedY = this.isFocused() ? y - BAR_HEIGHT : y;

    const i = Math.floor(computedY / PX_PER_LEVEL) + this.topLevel;

    if (i >= 0 && i < this.flamebearer.levels.length) {
      const j = this.binarySearchLevel(x, this.flamebearer.levels[i]);

      return { i, j };
    }

    return { i: 0, j: 0 };
  }

  reset() {
    this.selectedLevel = 0;
    this.topLevel = 0;
    this.rangeMin = 0;
    this.rangeMax = 1;
  }

  zoom(i: number, j: number) {
    const { ff } = this;

    this.selectedLevel = i;
    this.topLevel = 0;
    this.rangeMin =
      ff.getBarOffset(this.flamebearer.levels[i], j) /
      this.flamebearer.numTicks;
    this.rangeMax =
      (ff.getBarOffset(this.flamebearer.levels[i], j) +
        ff.getBarTotal(this.flamebearer.levels[i], j)) /
      this.flamebearer.numTicks;
  }

  isWithinBounds(x: number, y: number) {
    if (x < 0 || x > this.getCanvasWidth()) {
      return false;
    }

    try {
      const { i, j } = this.xyToBar(x, y);
      if (j === -1 || i === -1) {
        return false;
      }
    } catch (e) {
      return false;
    }

    return true;
  }
}
