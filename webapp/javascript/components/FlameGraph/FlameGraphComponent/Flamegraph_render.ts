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
import { Flamebearer, addTicks } from '@models/flamebearer';
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
// there's a dependency cycle here but it should be fine
/* eslint-disable-next-line import/no-cycle */
import Flamegraph from './Flamegraph';

type CanvasRendererConfig = Flamebearer & {
  canvas: HTMLCanvasElement;
  focusedNode: ConstructorParameters<typeof Flamegraph>[2];
  fitMode: ConstructorParameters<typeof Flamegraph>[3];
  highlightQuery: ConstructorParameters<typeof Flamegraph>[4];
  zoom: ConstructorParameters<typeof Flamegraph>[5];

  /**
   * Used when zooming, values between 0 and 1.
   * For illustration, in a non zoomed state it has the value of 0
   */
  readonly rangeMin: number;
  /**
   * Used when zooming, values between 0 and 1.
   * For illustration, in a non zoomed state it has the value of 1
   */
  readonly rangeMax: number;
};

export default function RenderCanvas(props: CanvasRendererConfig) {
  const { canvas } = props;
  const { numTicks, sampleRate, zoom } = props;
  const { fitMode } = props;
  const { units } = props;
  const { rangeMin, rangeMax } = props;

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

  // TODO what does ff mean?
  const { format } = props;
  const ff = createFF(format);

  const { levels } = props;
  const { focusedNode } = props;

  // TODO
  // this shouldn't be needed
  //  if (focusedNode.i === -1) {
  //    focusedNode.i = 0;
  //  }
  //  if (focusedNode.j === -1) {
  //    focusedNode.j = 0;
  //  }
  //
  //  const focusMin =
  //    ff.getBarOffset(levels[focusedNode.i], focusedNode.j) / numTicks;
  //
  //  const focusMax =
  //    (ff.getBarOffset(levels[focusedNode.i], focusedNode.j) +
  //      ff.getBarTotal(levels[focusedNode.i], focusedNode.j)) /
  //    numTicks;
  //
  //  // in case we are focusing
  //  // if focus is set but rangemin is not
  //  // or zoom is smaller
  //  if (
  //    (focusMin !== 0 && rangeMin === 0) ||
  //    (focusMax !== 1 && rangeMax === 1) ||
  //    rangeMin < focusMin
  //  ) {
  //    rangeMin = focusMin;
  //    rangeMax = focusMax;
  //    console.log('focus min is smaller than rageMin');
  //  }
  const pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);

  //  const pxPerTick = graphWidth / numTicks / (focusMax - focusMin);

  //  console.log({
  //    focusedNode,
  //    focusMax,
  //    focusMin,
  //    rangeMax,
  //    rangeMin,
  //    pxPerTick,
  //  });
  //
  const ctx = canvas.getContext('2d');
  const selectedLevel = zoom.map((z) => z.i).getOrElse(0);

  //  const { topLevel } = props;
  //  TODO
  //  const topLevel = 0;
  const formatter = getFormatter(numTicks, sampleRate, units);

  const isFocused = focusedNode.isSome();

  const topLevel = focusedNode.map((f) => f.i).getOrElse(0);
  //    focusedNode.i < 0 ? 0 : focusedNode.i;

  const canvasHeight =
    PX_PER_LEVEL * (levels.length - topLevel) + (isFocused ? BAR_HEIGHT : 0);
  //  const canvasHeight = PX_PER_LEVEL * (levels.length - topLevel);
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
  if (isFocused) {
    const width = numTicks * pxPerTick;
    ctx.beginPath();
    ctx.rect(0, 0, numTicks * pxPerTick, BAR_HEIGHT);
    // TODO find a neutral color
    // TODO use getColor ?
    ctx.fillStyle = colorGreyscale(200, 1).rgb().string();
    ctx.fill();

    // TODO show the samples too?
    const shortName = focusedNode
      .map((f) => `total (${f.i - 1} level(s) collapsed)`)
      .getOrElse('total');

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

      //      console.log('functino', n);
      const x = tickToX(numTicks, rangeMin, pxPerTick, barIndex);
      //      const x = tickToX(numTicks, focusMin, pxPerTick, barIndex);
      const y = i * PX_PER_LEVEL + (isFocused ? BAR_HEIGHT : 0);

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
      //      console.log('function with name', n, 'is collapsed', collapsed);
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
      //      console.log('function', n, {
      //        sw,
      //        x,
      //        y,
      //        sh,
      //        canvasWidth: canvas.width,
      //        numTicks,
      //        rangeMin,
      //        pxPerTick,
      //        barIndex,
      //      });
      //
      /*******************************/
      /*      D r a w   R e c t      */
      /*******************************/
      const { spyName } = props;
      let leftTicks: number | undefined;
      let rightTicks: number | undefined;
      if (format === 'double') {
        leftTicks = props.leftTicks;
        rightTicks = props.rightTicks;
      }
      const color = getColor({
        format,
        level,
        j,
        // discount for the levels we skipped
        // otherwise it will dim out all nodes
        i: i + focusedNode.map((f) => f.i).getOrElse(0),
        //        i: i + (isFocused ? focusedNode.i : 0),
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

      const shortName = getFunctionName(names, j, format, level);
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
  format: CanvasRendererConfig['format'],
  level: number[]
) {
  const ff = createFF(format);
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
  const ff = createFF(cfg.format);

  // all above selected level should be dimmed
  const a = cfg.selectedLevel > cfg.i ? 0.33 : 1;

  // Collapsed
  if (cfg.collapsed) {
    return colorGreyscale(200, 0.66);
  }

  // Diff mode
  if (cfg.format === 'double') {
    const { leftRatio, rightRatio } = getRatios(
      cfg.format,
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
  barTicks: number
) {
  // barticks is euqal to the offset?
  //  console.log({
  //    barTicks,
  //    numTicks,
  //    rangeMin,
  //    pxPerTick,
  //    total: (barTicks - numTicks * rangeMin) * pxPerTick,
  //  });
  return (barTicks - numTicks * rangeMin) * pxPerTick;
}

function nodeIsInQuery(
  index: number,
  level: number[],
  names: string[],
  query: string
) {
  return names[level[index]].indexOf(query) >= 0;
}
