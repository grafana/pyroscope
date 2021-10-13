import { PX_PER_LEVEL, COLLAPSE_THRESHOLD, BAR_HEIGHT, GAP } from './constants';
import { formatPercent, getFormatter, Units } from '../../../util/format';
import { fitToCanvasRect } from '../../../util/fitMode';

export interface CanvasRendererConfig {
  //  context: CanvasRenderingContext2D;
  canvas: HTMLCanvasElement;
  numTicks: number;
  sampleRate: number;
  names: string[];
  levels: number[][];

  viewType: 'single' | 'diff';

  pxPerTick: number;

  topLevel: number;
  rangeMin: number;
  rangeMax: number;

  units: Units;
  fitMode: 'TAIL' | 'HEAD'; // TODO import from fitMode
}

// TODO
// this shouldn't really be a component
// so don't call it props
export function RenderCanvas(props: CanvasRendererConfig) {
  const { canvas } = props;
  const { numTicks, rangeMin, rangeMax, sampleRate } = props;
  const { fitMode } = props;
  const { units } = props;

  const graphWidth = canvas.clientWidth;
  // TODO: why is this needed? otherwise height is all messed up
  canvas.width = graphWidth;

  const pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);

  const ctx = canvas.getContext('2d');

  // TODO what does ff mean?
  const { viewType } = props;
  const ff = createFF(viewType);

  const { names, levels, topLevel } = props;
  const formatter = getFormatter(numTicks, sampleRate, units);

  for (let i = 0; i < levels.length - topLevel; i += 1) {
    const level = levels[topLevel + i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const barIndex = ff.getBarOffset(level, j);
      const numBarTicks = ff.getBarTotal(level, j);

      const x = tickToX(numTicks, rangeMin, pxPerTick, barIndex);
      const y = i * PX_PER_LEVEL;

      // merge very small blocks into big "collapsed" ones for performance
      const collapsed = numBarTicks * pxPerTick <= COLLAPSE_THRESHOLD;

      const sw = numBarTicks * pxPerTick - (collapsed ? 0 : GAP);
      const sh = BAR_HEIGHT;

      // Decide the name
      const ratio = numBarTicks / numTicks;

      const percent = formatPercent(ratio);
      const shortName = names[level[j + ff.jName]];
      const longName = `${shortName} (${percent}, ${formatter.format(
        numBarTicks,
        sampleRate
      )})`;

      const namePosX = Math.round(Math.max(x, 0));

      // It's important to set the font before hand
      // Since it will be used to calculate how many characters can fit
      ctx.textBaseline = 'middle';
      ctx.font =
        '400 11.5px SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace';
      // Since this is a monospaced font any character would do
      const characterSize = ctx.measureText('a').width;

      const fitCalc = fitToCanvasRect({
        mode: fitMode,
        charSize: characterSize,
        rectWidth: sw,
        fullText: longName,
        shortText: shortName,
      });

      // Draw the block
      ctx.beginPath();
      ctx.rect(x, y, sw, sh);
      // TODO color
      ctx.fillStyle = '#48CE73'; // green
      ctx.fill();

      ctx.save();
      ctx.clip();
      ctx.fillStyle = 'black';
      // when showing the code, give it a space in the beginning
      ctx.fillText(fitCalc.text, namePosX + fitCalc.marginLeft, y + sh / 2 + 1);
      ctx.restore();
    }
  }
}

function createFF(viewType: CanvasRendererConfig['viewType']) {
  switch (viewType) {
    case 'single': {
      return formatSingle;
    }
    case 'diff': {
      return formatDouble;
    }

    default:
      throw new Error(`Format not supported: '${viewType}'`);
  }
}

const formatSingle = {
  format: 'single',
  jStep: 4,
  jName: 3,
  getBarOffset: (level: number[], j: number) => level[j],
  getBarTotal: (level: number[], j: number) => level[j + 1],
  getBarTotalDiff: (level: number[], j: number) => 0,
  getBarSelf: (level: number[], j: number) => level[j + 2],
  getBarSelfDiff: (level: number[], j: number) => 0,
  getBarName: (level: number[], j: number) => level[j + 3],
};

const formatDouble = {
  format: 'double',
  jStep: 7,
  jName: 6,
  getBarOffset: (level: number[], j: number) => level[j] + level[j + 3],
  getBarTotal: (level: number[], j: number) => level[j + 4] + level[j + 1],
  getBarTotalLeft: (level: number[], j: number) => level[j + 1],
  getBarTotalRght: (level: number[], j: number) => level[j + 4],
  getBarTotalDiff: (level: number[], j: number) => {
    return level[j + 4] - level[j + 1];
  },
  getBarSelf: (level: number[], j: number) => level[j + 5] + level[j + 2],
  getBarSelfLeft: (level: number[], j: number) => level[j + 2],
  getBarSelfRght: (level: number[], j: number) => level[j + 5],
  getBarSelfDiff: (level: number[], j: number) => level[j + 5] - level[j + 2],
  getBarName: (level: number[], j: number) => level[j + 6],
};

function tickToX(
  numTicks: number,
  rangeMin: number,
  pxPerTick: number,
  i: number
) {
  return (i - numTicks * rangeMin) * pxPerTick;
}
