import { PX_PER_LEVEL, COLLAPSE_THRESHOLD, BAR_HEIGHT, GAP } from './constants';
import {
  formatPercent,
  getFormatter,
  Units,
  ratioToPercent,
} from '../../../util/format';
import { fitToCanvasRect } from '../../../util/fitMode';
import { createFF, getRatios } from './utils';
import {
  colorBasedOnDiff,
  colorBasedOnDiffPercent,
  colorBasedOnPackageName,
  colorFromPercentage,
  colorGreyscale,
  diffColorGreen,
  diffColorRed,
  getPackageNameFromStackTrace,
} from './color';

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

  canvasWidth?: number;
}

// TODO
// this shouldn't really be a component
// so don't call it props
export function RenderCanvas(props: CanvasRendererConfig) {
  const { canvas } = props;
  const { numTicks, rangeMin, rangeMax, sampleRate } = props;
  const { fitMode } = props;
  const { units } = props;

  const graphWidth = canvas.clientWidth || props.canvasWidth;
  // TODO: why is this needed? otherwise height is all messed up
  canvas.width = graphWidth;

  if (rangeMin >= rangeMax) {
    throw new Error(`'rangeMin' should be strictly smaller than 'rangeMax'`);
  }
  const pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);

  const ctx = canvas.getContext('2d');

  // TODO what does ff mean?
  const { viewType } = props;
  const ff = createFF(viewType);

  const { names, levels, topLevel } = props;
  const formatter = getFormatter(numTicks, sampleRate, units);

  // Set the font syle
  // It's important to set the font before hand
  // Since it will be used to calculate how many characters can fit
  ctx.textBaseline = 'middle';
  ctx.font =
    '400 11.5px SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace';
  // Since this is a monospaced font any character would do
  const characterSize = ctx.measureText('a').width;

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

      /*********************/
      /*      N a m e      */
      /*********************/
      const shortName = getFunctionName(names, j, viewType, level);
      const longName = getLongName(
        shortName,
        numBarTicks,
        numTicks,
        sampleRate,
        formatter
      );

      const fitCalc = fitToCanvasRect({
        mode: fitMode,
        charSize: characterSize,
        rectWidth: sw,
        fullText: longName,
        shortText: shortName,
      });

      /*********************/
      /*      D r a w      */
      /*********************/
      ctx.beginPath();
      ctx.rect(x, y, sw, sh);

      const color = getColor({
        viewType,
        level,
        j,
        i,
        names,
        // TODO
        collapsed: false,
        selectedLevel: 0,
        queryExists: false,
        nodeIsInQuery: false,
        spyName: 'gospy',
      });

      // hex is necessary for node-canvas (and therefore tests) to work
      // bear in mind this is pure conjecture
      ctx.fillStyle = color.hex();
      ctx.fill();

      ctx.save();
      ctx.clip();
      ctx.fillStyle = 'black';

      const namePosX = Math.round(Math.max(x, 0));
      ctx.fillText(fitCalc.text, namePosX + fitCalc.marginLeft, y + sh / 2 + 1);
      ctx.restore();
    }
  }
}

function getFunctionName(
  names: CanvasRendererConfig['names'],
  j: number,
  viewType: CanvasRendererConfig['viewType'],
  level: number[]
) {
  const ff = createFF(viewType);
  const shortName = names[level[j + ff.jName]];
  return shortName;
}

function getLongName(
  shortName: string,
  numBarTicks: number,
  numTicks: number,
  sampleRate: number,
  formatter: ReturnType<typeof getFormatter>
) {
  const ratio = numBarTicks / numTicks;
  const percent = formatPercent(ratio);

  const longName = `${shortName} (${percent}, ${formatter.format(
    numBarTicks,
    sampleRate
  )})`;

  return longName;
}

// TODO use dependant types
function getColor({
  viewType,
  collapsed,
  level,
  j,
  leftTicks,
  rightTicks,
  selectedLevel,
  i,
  queryExists,
  nodeIsInQuery,
  names,
  spyName,
}: {
  viewType: CanvasRendererConfig['viewType'];
  collapsed: boolean;
  level: number[];
  j: number;
  leftTicks?: number;
  rightTicks?: number;
  selectedLevel: number;
  i: number;
  queryExists: boolean;
  nodeIsInQuery: boolean;
  names: string[];
  spyName: string;
}) {
  const HIGHLIGHT_NODE_COLOR = '#48CE73'; // green
  const ff = createFF(viewType);

  // all above selected level should be dimmed
  const a = selectedLevel > i ? 0.33 : 1;

  // Collapsed
  if (collapsed) {
    return colorGreyscale(200, 0.66);
  }

  // Diff mode
  if (viewType === 'diff') {
    const { leftRatio, rightRatio } = getRatios(
      viewType,
      level,
      j,
      leftTicks,
      rightTicks
    );

    const leftPercent = ratioToPercent(leftRatio);
    const rightPercent = ratioToPercent(rightRatio);

    return colorBasedOnDiffPercent(leftPercent, rightPercent, a);
  }

  // We are in a search
  if (queryExists) {
    if (nodeIsInQuery) {
      return HIGHLIGHT_NODE_COLOR;
    }
    return colorGreyscale(200, 0.66);
  }

  return colorBasedOnPackageName(
    getPackageNameFromStackTrace(spyName, names[level[j + ff.jName]]),
    a
  );
}

function tickToX(
  numTicks: number,
  rangeMin: number,
  pxPerTick: number,
  i: number
) {
  return (i - numTicks * rangeMin) * pxPerTick;
}
