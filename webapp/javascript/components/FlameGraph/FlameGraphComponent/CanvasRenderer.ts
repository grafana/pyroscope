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
import { getRatios } from './utils';
import { createFF } from '../../../util/flamebearer';
import {
  colorBasedOnDiffPercent,
  colorBasedOnPackageName,
  colorGreyscale,
  getPackageNameFromStackTrace,
  highlightColor,
} from './color';

export type CanvasRendererConfig = {
  canvas: HTMLCanvasElement;
  numTicks: number;

  /**
   * Sample Rate, used in text information
   */
  sampleRate: number;

  /**
   * List of names
   */
  names: string[];

  /**
   * List of level
   *
   * This is NOT the same as in the flamebearer
   * that we receive from the server.
   * As in there are some transformations required
   * (see deltaDiffWrapper)
   */
  levels: number[][];

  /**
   * What level to start from
   */
  topLevel: number;

  /**
   * Used when zooming, values between 0 and 1.
   * For illustration, in a non zoomed in state it has the value of 0
   */
  rangeMin: number;
  /**
   * Used when zooming, values between 0 and 1.
   * For illustration, in a non zoomed in state it has the value of 1
   */
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
   * What level has been "selected"
   * All nodes above will be dimmed out
   */
  selectedLevel?: number;
} & addTicks;

// if it's type double (diff), we also require `left` and `right` ticks
type addTicks =
  | { viewType: 'double'; leftTicks: number; rightTicks: number }
  | { viewType: 'single' };

export function RenderCanvas(props: CanvasRendererConfig) {
  const { canvas } = props;
  const { numTicks, rangeMin, rangeMax, sampleRate } = props;
  const { fitMode } = props;
  const { units } = props;

  // clientWidth includes padding
  // however it's not present in node-canvas (used for testing)
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

  const { topLevel } = props;
  const formatter = getFormatter(numTicks, sampleRate, units);

  const { levels } = props;

  let focused = false;
  if (topLevel > 0) {
    focused = true;
  }

  const canvasHeight =
    PX_PER_LEVEL * (levels.length - topLevel) + (focused ? BAR_HEIGHT : 0);
  canvas.height = canvasHeight;

  // increase pixel ratio, otherwise it looks bad in high resolution devices
  if (devicePixelRatio > 1) {
    canvas.width *= 2;
    canvas.height *= 2;
    ctx.scale(2, 2);
  }

  const { names } = props;
  // are we focused?
  // if so, add an initial bar telling it's a collapsed one
  // TODO clean this up
  if (focused) {
    const width = numTicks * pxPerTick;
    ctx.beginPath();
    ctx.rect(0, 0, numTicks * pxPerTick, BAR_HEIGHT);
    // TODO find a neutral color
    ctx.fillStyle = 'grey';
    ctx.fill();

    const shortName = `total (${topLevel} level(s) skipped)`;

    // Set the font syle
    // It's important to set the font BEFORE calculating 'characterSize'
    // Since it will be used to calculate how many characters can fit
    ctx.textBaseline = 'middle';
    ctx.font =
      '400 11.5px SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace';
    // Since this is a monospaced font any character would do
    const characterSize = ctx.measureText('a').width;
    const fitCalc = fitToCanvasRect({
      mode: fitMode,
      charSize: characterSize,
      rectWidth: width,
      fullText: shortName,
      shortText: shortName,
    });

    const x = 0;
    const y = 0;
    const sh = BAR_HEIGHT;

    ctx.save();
    ctx.clip();
    ctx.fillStyle = 'black';
    const namePosX = Math.round(Math.max(x, 0));
    ctx.fillText(fitCalc.text, namePosX + fitCalc.marginLeft, y + sh / 2 + 1);
    ctx.restore();
  }

  for (let i = 0; i < levels.length - topLevel; i += 1) {
    const level = levels[topLevel + i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const barIndex = ff.getBarOffset(level, j);

      const x = tickToX(numTicks, rangeMin, pxPerTick, barIndex);
      const y = i * PX_PER_LEVEL + (focused ? BAR_HEIGHT : 0);

      const sh = BAR_HEIGHT;

      const highlightModeOn =
        props.highlightQuery && props.highlightQuery.length > 0;
      const isHighlighted = nodeIsInQuery(
        j + ff.jName,
        level,
        names,
        props.highlightQuery
      );

      let numBarTicks = ff.getBarTotal(level, j);

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

      /*******************************/
      /*      D r a w   R e c t      */
      /*******************************/
      const { spyName } = props;
      let leftTicks: number | undefined;
      if (props.viewType === 'double') {
        leftTicks = props.leftTicks;
      }
      let rightTicks: number | undefined;
      if (props.viewType === 'double') {
        rightTicks = props.rightTicks;
      }
      const color = getColor({
        viewType,
        level,
        j,
        i,
        names,
        collapsed,
        selectedLevel,
        highlightModeOn,
        isHighlighted,
        spyName,
        leftTicks,
        rightTicks,
      });

      ctx.beginPath();
      ctx.rect(x, y, sw, sh);
      ctx.fillStyle = color.string();
      ctx.fill();

      /*******************************/
      /*      D r a w   T e x t      */
      /*******************************/
      // don't write text if there's not enough space for a single letter
      if (collapsed) {
        continue;
      }

      if (sw < LABEL_THRESHOLD) {
        continue;
      }

      const shortName = getFunctionName(names, j, viewType, level);
      const longName = getLongName(
        shortName,
        numBarTicks,
        numTicks,
        sampleRate,
        formatter
      );

      // Set the font syle
      // It's important to set the font BEFORE calculating 'characterSize'
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

type getColorCfg = {
  collapsed: boolean;
  level: number[];
  j: number;
  selectedLevel: number;
  i: number;
  highlightModeOn: boolean;
  isHighlighted: boolean;
  names: string[];
  spyName: string;
} & addTicks;

function getColor(cfg: getColorCfg) {
  const ff = createFF(cfg.viewType);

  // all above selected level should be dimmed
  const a = cfg.selectedLevel > cfg.i ? 0.33 : 1;

  // Collapsed
  if (cfg.collapsed) {
    return colorGreyscale(200, 0.66);
  }

  // Diff mode
  if (cfg.viewType === 'double') {
    const { leftRatio, rightRatio } = getRatios(
      cfg.viewType,
      cfg.level,
      cfg.j,
      cfg.leftTicks,
      cfg.rightTicks
    );

    const leftPercent = ratioToPercent(leftRatio);
    const rightPercent = ratioToPercent(rightRatio);

    return colorBasedOnDiffPercent(leftPercent, rightPercent, a);
  }

  // We are in a search
  if (cfg.highlightModeOn) {
    if (cfg.isHighlighted) {
      return highlightColor;
    }
    return colorGreyscale(200, 0.66);
  }

  return colorBasedOnPackageName(
    getPackageNameFromStackTrace(
      cfg.spyName,
      cfg.names[cfg.level[cfg.j + ff.jName]]
    ),
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
