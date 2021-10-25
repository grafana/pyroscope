import { createFF } from '@utils/flamebearer';
import { Flamebearer } from '@models/flamebearer';
import { PX_PER_LEVEL, BAR_HEIGHT, COLLAPSE_THRESHOLD } from './constants';
// there's a dependency cycle here but it should be fine
/* eslint-disable-next-line import/no-cycle */
import RenderCanvas from './Flamegraph_render';

/* eslint-disable no-useless-constructor */

export default class Flamegraph {
  private ff: ReturnType<typeof createFF>;

  // used in zoom
  private rangeMin: number;

  // used in zoom
  private rangeMax: number;

  constructor(
    private readonly flamebearer: Flamebearer,
    private canvas: HTMLCanvasElement,
    /**
     * What node to be 'focused'
     * ie what node to start the tree
     */
    private focusedNode: { i: number; j: number },
    /**
     * What level has been "selected"
     * All nodes above will be dimmed out
     */
    //    private selectedLevel: number,
    private fitMode: 'HEAD' | 'TAIL',
    /**
     * The query used to match against the node name.
     * For each node,
     * if it matches it will be highlighted,
     * otherwise it will be greyish.
     */
    private highlightQuery: string,
    private zoom: { i: number; j: number }
  ) {
    this.ff = createFF(flamebearer.format);

    this.setupZoom(zoom.i, zoom.j, flamebearer);
  }

  private setupZoom(i: number, j: number, flamebearer: Flamebearer) {
    const { ff } = this;

    // no zoom
    if (i === -1 || j === -1) {
      this.rangeMin = 0;
      this.rangeMax = 1;
      //      this.selectedLevel = 0;
      //     this.topLevel = 0;
      return;
    }

    //    this.topLevel = 0;
    //  this.selectedLevel = i;
    this.rangeMin =
      ff.getBarOffset(flamebearer.levels[i], j) / flamebearer.numTicks;
    this.rangeMax =
      (ff.getBarOffset(flamebearer.levels[i], j) +
        ff.getBarTotal(flamebearer.levels[i], j)) /
      this.flamebearer.numTicks;
  }

  public render() {
    const props = {
      canvas: this.canvas,

      format: this.flamebearer.format,
      numTicks: this.flamebearer.numTicks,
      sampleRate: this.flamebearer.sampleRate,
      names: this.flamebearer.names,
      levels: this.flamebearer.levels,
      spyName: this.flamebearer.spyName,
      units: this.flamebearer.units,

      //      topLevel: this.topLevel,
      rangeMin: this.rangeMin,
      rangeMax: this.rangeMax,
      fitMode: this.fitMode,
      highlightQuery: this.highlightQuery,
      zoom: this.zoom,
      focusedNode: this.focusedNode,
    };

    const { format: viewType } = this.flamebearer;

    switch (viewType) {
      case 'single': {
        RenderCanvas({ ...props, format: 'single' });
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
        throw new Error(`Invalid format: '${viewType}'`);
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
    if (this.focusedNode.i === -1 && this.focusedNode.j === -1) {
      return false;
    }
    if (this.focusedNode.i === 0 && this.focusedNode.j === 0) {
      return false;
    }

    return true;
  }

  // binary search of a block in a stack level
  // TODO(eh-am): calculations seem wrong when x is 0 and y != 0,
  // also when on the border
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

  public xyToBar(x: number, y: number) {
    if (x < 0 || y < 0) {
      throw new Error(`x and y must be bigger than 0. x = ${x}, y = ${y}`);
    }

    // in focused mode there's a "fake" bar at the top
    // so we must discount for it
    const computedY = this.isFocused() ? y - BAR_HEIGHT : y;

    // TODO
    // shouldn't have to normalize the zoom here
    const i =
      Math.floor(computedY / PX_PER_LEVEL) +
      (this.zoom.i <= 0 ? 0 : this.zoom.i);

    if (i >= 0 && i < this.flamebearer.levels.length) {
      const j = this.binarySearchLevel(x, this.flamebearer.levels[i]);

      return { i, j };
    }

    return { i: 0, j: 0 };
  }

  public isWithinBounds = (x: number, y: number) => {
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
  };

  /*
   * Given x and y coordinates
   * identify the bar position and width
   *
   * This can be used for highlighting
   */
  public xyToBarPosition = (x: number, y: number) => {
    if (!this.isWithinBounds(x, y)) {
      throw new Error(
        `Value out of bounds. Can't get bar position x:'${x}', y:'${y}'`
      );
    }

    const { ff } = this;
    const { i, j } = this.xyToBar(x, y);

    const level = this.flamebearer.levels[i];

    const posX = Math.max(this.tickToX(ff.getBarOffset(level, j)), 0);
    // TODO shouldn't have to normalize this
    const posY =
      (i - (this.zoom.i <= 0 ? 0 : this.zoom.i)) * PX_PER_LEVEL +
      (this.isFocused() ? BAR_HEIGHT : 0);

    const sw = Math.min(
      this.tickToX(ff.getBarOffset(level, j) + ff.getBarTotal(level, j)) - posX,
      this.getCanvasWidth()
    );

    return {
      x: posX,
      y: posY,
      width: sw,
    };
  };

  // TODO maybe we should combine xyToBarPosition with this?
  /*
   * Given x and y coordinates
   * return all node data available in the flamegraph
   */
  public xyToBarData(x: number, y: number) {
    if (!this.isWithinBounds(x, y)) {
      throw new Error(
        `Value out of bounds. Can't get bar position. x: '${x}', y: '${y}'`
      );
    }

    const { i, j } = this.xyToBar(x, y);
    const level = this.flamebearer.levels[i];

    const { ff } = this;

    switch (this.flamebearer.format) {
      case 'single': {
        return {
          format: 'single' as const,
          name: this.flamebearer.names[ff.getBarName(level, j)],
          self: ff.getBarSelf(level, j),
          offset: ff.getBarOffset(level, j),
          total: ff.getBarTotal(level, j),
        };
      }
      case 'double': {
        return {
          format: 'double' as const,
          barTotal: ff.getBarTotal(level, j),
          totalLeft: ff.getBarTotalLeft(level, j),
          totalRight: ff.getBarTotalRght(level, j),
          totalDiff: ff.getBarTotalDiff(level, j),
          name: this.flamebearer.names[ff.getBarName(level, j)],
        };
      }

      default: {
        throw new Error(`Unsupported type`);
      }
    }
  }
}
