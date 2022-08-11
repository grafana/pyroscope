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
import { createFF, Flamebearer, SpyName } from '@pyroscope/models/src';
import {
  formatPercent,
  getFormatter,
  ratioToPercent,
} from '../../format/format';
import { fitToCanvasRect } from '../../fitMode/fitMode';
import { getRatios } from './utils';
import {
  PX_PER_LEVEL,
  COLLAPSE_THRESHOLD,
  LABEL_THRESHOLD,
  BAR_HEIGHT,
  GAP,
} from './constants';
import {
  colorBasedOnDiffPercent,
  colorBasedOnPackageName,
  colorGreyscale,
  getPackageNameFromStackTrace,
} from './color';
import type { FlamegraphPalette } from './colorPalette';
import { isMatch } from '../../search';
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

  tickToX: (i: number) => number;

  pxPerTick: number;

  palette: FlamegraphPalette;
  maxSelf?: number;
};

export default function RenderCanvas(props: CanvasRendererConfig) {
  const { canvas, fitMode, units, tickToX, levels, palette } = props;
  const { numTicks, sampleRate, pxPerTick } = props;
  const { rangeMin, rangeMax } = props;
  const { focusedNode, zoom } = props;

  const graphWidth = getCanvasWidth(canvas);
  // TODO: why is this needed? otherwise height is all messed up
  canvas.width = graphWidth;

  if (rangeMin >= rangeMax) {
    throw new Error(`'rangeMin' should be strictly smaller than 'rangeMax'`);
  }

  const { format } = props;
  const ff = createFF(format);

  //  const pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    throw new Error('Could not get ctx');
  }

  const selectedLevel = zoom.mapOrElse(
    () => 0,
    (z) => z.i
  );
  const formatter = getFormatter(numTicks, sampleRate, units);
  const isFocused = focusedNode.isJust;
  const topLevel = focusedNode.mapOrElse(
    () => 0,
    (f) => f.i
  );

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
    const shortName = focusedNode.mapOrElse(
      () => 'total',
      (f) => `total (${f.i - 1} level(s) collapsed)`
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
    if (!level) {
      throw new Error(`Could not find level: ${topLevel + i}`);
    }

    for (let j = 0; j < level.length; j += ff.jStep) {
      const barIndex = ff.getBarOffset(level, j);
      const x = tickToX(barIndex);
      const y = i * PX_PER_LEVEL + (isFocused ? BAR_HEIGHT : 0);

      const sh = BAR_HEIGHT;

      const highlightModeOn =
        !!props.highlightQuery && props.highlightQuery.length > 0;

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

      const getColor = () => {
        const common = {
          level,
          j,
          // discount for the levels we skipped
          // otherwise it will dim out all nodes
          i:
            i +
            focusedNode.mapOrElse(
              () => 0,
              (f) => f.i
            ),
          names,
          collapsed,
          selectedLevel,
          highlightModeOn,
          isHighlighted,
          // keep type narrow https://stackoverflow.com/q/54333982
          // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
          spyName: spyName as SpyName,
          palette,
        };

        switch (format) {
          case 'single': {
            return getColorSingle({ ...common });
          }
          case 'double': {
            return getColorDouble({
              ...common,
              leftTicks: props.leftTicks,
              rightTicks: props.rightTicks,
            });
          }
          default: {
            throw new Error(`Unsupported format: ${format}`);
          }
        }
      };

      const color = getColor();

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

  let l = level[j + ff.jName];
  if (l === undefined) {
    l = -1;
  }
  const shortName = names[l];

  if (!shortName) {
    console.warn('Could not find function name for', {
      j,
      format,
      level,
      names,
    });
    return '';
  }
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
  spyName: SpyName;
  palette: FlamegraphPalette;
};

function getColorCommon({
  collapsed,
  highlightModeOn,
  isHighlighted,
}: Pick<
  getColorCfg,
  'selectedLevel' | 'i' | 'collapsed' | 'highlightModeOn' | 'isHighlighted'
>) {
  // Collapsed
  if (collapsed) {
    return colorGreyscale(200, 0.66);
  }

  // We are in a search
  if (highlightModeOn) {
    if (!isHighlighted) {
      return colorGreyscale(200, 0.66);
    }
  }

  return null;
}

function getColorSingle(cfg: getColorCfg) {
  const common = getColorCommon(cfg);

  // common cases, like highlight
  if (common) {
    return common;
  }

  const ff = createFF('single');

  const a = cfg.selectedLevel > cfg.i ? 0.33 : 1;

  // TODO: clean this up
  let l = cfg.level[cfg.j + ff.jName];
  if (l === undefined) {
    console.warn('Could nto find level', {
      l: cfg.j,
      jName: ff.jName,
      level: cfg.level,
    });
    l = -1;
  }
  const name = cfg.names[l] || '';
  const packageName = getPackageNameFromStackTrace(cfg.spyName, name) || '';

  return colorBasedOnPackageName(cfg.palette, packageName).alpha(a);
}

function getColorDouble(
  cfg: getColorCfg & { leftTicks: number; rightTicks: number }
) {
  const common = getColorCommon(cfg);

  // common cases, like highlight
  if (common) {
    return common;
  }

  const a = cfg.selectedLevel > cfg.i ? 0.33 : 1;
  const { leftRatio, rightRatio } = getRatios(
    cfg.level,
    cfg.j,
    cfg.leftTicks,
    cfg.rightTicks
  );

  const leftPercent = ratioToPercent(leftRatio);
  const rightPercent = ratioToPercent(rightRatio);

  return colorBasedOnDiffPercent(cfg.palette, leftPercent, rightPercent).alpha(
    a
  );
}

function nodeIsInQuery(
  index: number,
  level: number[],
  names: string[],
  query: string
) {
  const l = level[index];
  if (!l) {
    return false;
  }

  const l2 = names[l];
  if (!l2) {
    return false;
  }

  return isMatch(query, l2);
}

function getCanvasWidth(canvas: HTMLCanvasElement) {
  // clientWidth includes padding
  // however it's not present in node-canvas (used for testing)
  // so we also fallback to canvas.width
  return canvas.clientWidth || canvas.width;
}
