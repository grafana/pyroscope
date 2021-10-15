/*

This component is based on code from flamebearer project
  https://github.com/mapbox/flamebearer

ISC License

Copyright (c) 2018, Mapbox

Permission to use, copy, modify, and/or distribute this software for any purpose
with or without fee is hereby granted, provided that the above copyright notice
and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.

*/

/* eslint-disable no-continue */
import {
  PX_PER_LEVEL,
  COLLAPSE_THRESHOLD,
  LABEL_THRESHOLD,
  BAR_HEIGHT,
  GAP,
} from './constants';
import {
  formatPercent,
  getFormatter,
  Units,
  ratioToPercent,
} from '../../../util/format';
import { fitToCanvasRect } from '../../../util/fitMode';
import { createFF, getRatios } from './utils';
import {
  colorBasedOnDiffPercent,
  colorBasedOnPackageName,
  colorGreyscale,
  getPackageNameFromStackTrace,
  highlightColor,
} from './color';

export interface CanvasRendererConfig {
  canvas: HTMLCanvasElement;
  numTicks: number;
  sampleRate: number;
  names: string[];

  /**
   * It's important to remember that this is NOT the same flamebearer
   * that we receive from the server.
   * As in there are some transformations required
   * (see deltaDiffWrapper)
   */
  levels: number[][];

  viewType: 'single' | 'diff';

  topLevel: number;
  rangeMin: number;
  rangeMax: number;

  units: Units;
  fitMode: 'TAIL' | 'HEAD'; // TODO import from fitMode

  /**
   * The query used to match against the node name.
   * For each node,
   * if it matches it will be highlighted,
   * otherwise it will be greyish.
   */
  highlightQuery?: string;

  // needed in CI
  font?: string;

  // TODO type this
  spyName:
    | 'dotneyspy'
    | 'ebpfspy'
    | 'gospy'
    | 'phpspy'
    | 'pyspy'
    | 'rbspy'
    | string;

  /**
   * What level has been "selected" (TODO: find a better name)
   * All nodes above will be dimmed out
   */
  selectedLevel?: number;

  leftTicks?: number;
  rightTicks?: number;
}

// TODO
// this shouldn't really be a component
// so don't call it props
export function RenderCanvas(props: CanvasRendererConfig) {
  const { canvas } = props;
  const { numTicks, rangeMin, rangeMax, sampleRate } = props;
  const { fitMode } = props;
  const { units } = props;

  const { leftTicks, rightTicks } = props;

  //  console.log('canvas', JSON.stringify(Object.keys(obj)canvas));
  //  Object.keys(canvas).forEach((prop) => console.log(prop));

  // clientWidth includes padding
  // however it's not present in node-canvas
  // so we also fallback to canvas.width
  const graphWidth = canvas.clientWidth || canvas.width;
  if (!graphWidth) {
    throw new Error(
      `Could not infer canvasWidth. Tried 'canvas.clientWidth' and 'canvas.width'`
    );
  }

  // TODO: why is this needed? otherwise height is all messed up
  canvas.width = graphWidth;

  if (rangeMin >= rangeMax) {
    throw new Error(`'rangeMin' should be strictly smaller than 'rangeMax'`);
  }
  const pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);

  const ctx = canvas.getContext('2d');
  const { selectedLevel } = props;

  // TODO what does ff mean?
  const { viewType } = props;
  const ff = createFF(viewType);

  const { names, levels, topLevel } = props;
  const formatter = getFormatter(numTicks, sampleRate, units);

  // Set the font syle
  // It's important to set the font before hand
  // Since it will be used to calculate how many characters can fit
  ctx.textBaseline = 'middle';
  ctx.font = props.font
    ? props.font
    : '400 11.5px SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace';
  // Since this is a monospaced font any character would do
  const characterSize = ctx.measureText('a').width;

  // setup height
  //    this.canvas.height = this.props.height
  //      ? this.props.height - 30
  //      : PX_PER_LEVEL * (levels.length - this.topLevel);
  //
  const canvasHeight = PX_PER_LEVEL * (levels.length - topLevel);
  canvas.height = canvasHeight;
  // not sure this is required
  //  canvas.style.height = `${canvasHeight}px`;

  for (let i = 0; i < levels.length - topLevel; i += 1) {
    const level = levels[topLevel + i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const barIndex = ff.getBarOffset(level, j);

      let numBarTicks = ff.getBarTotal(level, j);

      const x = tickToX(numTicks, rangeMin, pxPerTick, barIndex);
      const y = i * PX_PER_LEVEL;

      const sh = BAR_HEIGHT;

      // Highlight stuff
      const highlightModeOn =
        props.highlightQuery && props.highlightQuery.length > 0;

      const isHighlighted = nodeIsInQuery(
        j + ff.jName,
        level,
        names,
        props.highlightQuery
      );

      // merge very small blocks into big "collapsed" ones for performance
      const collapsed = numBarTicks * pxPerTick <= COLLAPSE_THRESHOLD;
      if (collapsed) {
        // TODO: refactor this
        while (
          j < level.length - ff.jStep &&
          barIndex + numBarTicks === ff.getBarOffset(level, j + ff.jStep) &&
          ff.getBarTotal(level, j + ff.jStep) * pxPerTick <=
            COLLAPSE_THRESHOLD &&
          isHighlighted ===
            ((props.highlightQuery &&
              nodeIsInQuery(
                j + ff.jStep + ff.jName,
                level,
                names,
                props.highlightQuery
              )) ||
              false)
        ) {
          j += ff.jStep;
          numBarTicks += ff.getBarTotal(level, j);
        }
      }

      const sw = numBarTicks * pxPerTick - (collapsed ? 0 : GAP);

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

      const { spyName } = props;
      const color = getColor({
        viewType,
        level,
        j,
        i,
        names,
        // TODO
        collapsed,
        selectedLevel,
        highlightModeOn,
        isHighlighted,
        spyName,
        leftTicks,
        rightTicks,
      });

      ctx.fillStyle = color.string();
      ctx.fill();

      // don't write text if there's not enough space for a single letter
      if (collapsed) {
        continue;
      }

      if (sw < LABEL_THRESHOLD) {
        continue;
      }

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
  highlightModeOn,
  isHighlighted,
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
  highlightModeOn: boolean;
  isHighlighted: boolean;
  names: string[];
  spyName: string;
}) {
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
  if (highlightModeOn) {
    if (isHighlighted) {
      return highlightColor;
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

function nodeIsInQuery(
  index: number,
  level: number[],
  names: string[],
  query: string
) {
  return names[level[index]].indexOf(query) >= 0;
}
