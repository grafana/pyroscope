import React, { useCallback, RefObject, Dispatch, SetStateAction } from 'react';
import { Maybe } from 'true-myth';
import type { Unwrapped } from 'true-myth/maybe';
import { Units } from '@pyroscope/legacy/models/units';
import {
  getFormatter,
  numberWithCommas,
  formatPercent,
  ratioToPercent,
  diffPercent,
} from '../format/format';
import {
  FlamegraphPalette,
  DefaultPalette,
} from '../FlameGraph/FlameGraphComponent/colorPalette';

import { Tooltip, TooltipData } from './Tooltip';

type xyToDataSingle = (
  x: number,
  y: number
) => Maybe<{ format: 'single'; name: string; total: number }>;

type xyToDataDouble = (
  x: number,
  y: number
) => Maybe<{
  format: 'double';
  name: string;
  totalLeft: number;
  totalRight: number;
  barTotal: number;
}>;

export type FlamegraphTooltipProps = {
  canvasRef: RefObject<HTMLCanvasElement>;

  units: Units;
  sampleRate: number;
  numTicks: number;
  leftTicks: number;
  rightTicks: number;

  palette: FlamegraphPalette;
} & (
  | { format: 'single'; xyToData: xyToDataSingle }
  | {
      format: 'double';
      leftTicks: number;
      rightTicks: number;
      xyToData: xyToDataDouble;
    }
);

export default function FlamegraphTooltip(props: FlamegraphTooltipProps) {
  const {
    format,
    canvasRef,
    xyToData,
    numTicks,
    sampleRate,
    units,
    leftTicks,
    rightTicks,
    palette,
  } = props;

  const setTooltipContent = useCallback(
    (
      setContent: Dispatch<
        SetStateAction<{
          title: {
            text: string;
            diff: {
              text: string;
              color: string;
            };
          };
          tooltipData: TooltipData[];
        }>
      >,
      onMouseOut: () => void,
      e: MouseEvent
    ) => {
      const formatter = getFormatter(numTicks, sampleRate, units);
      const opt = xyToData(e.offsetX, e.offsetY);

      let data: Unwrapped<typeof opt>;

      // waiting on
      // https://github.com/true-myth/true-myth/issues/279
      if (opt.isJust) {
        data = opt.value;
      } else {
        onMouseOut();
        return;
      }

      // set the content for tooltip
      switch (data.format) {
        case 'single': {
          const newLeftContent: TooltipData = {
            percent: formatPercent(data.total / numTicks),
            samples:
              units === 'trace_samples' ? '' : numberWithCommas(data.total),
            units,
            formattedValue: formatter.format(data.total, sampleRate),
            tooltipType: 'flamegraph',
          };
          setContent({
            title: {
              text: data.name,
              diff: {
                text: '',
                color: '',
              },
            },
            tooltipData: [newLeftContent],
          });

          break;
        }

        case 'double': {
          if (format === 'single') {
            throw new Error(
              "props format is 'single' but it has been mapped to 'double'"
            );
          }

          const d = formatDouble(
            {
              formatter,
              sampleRate,
              totalLeft: data.totalLeft,
              leftTicks,
              totalRight: data.totalRight,
              rightTicks,
              title: data.name,
              units,
            },
            palette
          );

          setContent({
            title: d.title,
            tooltipData: d.tooltipData,
          });

          break;
        }
        default:
          throw new Error(`Unsupported format:'`);
      }
    },
    [
      numTicks,
      sampleRate,
      units,
      leftTicks,
      rightTicks,
      palette,
      format,
      xyToData,
    ]
  );

  return (
    <Tooltip
      dataSourceRef={canvasRef}
      clickInfoSide="right"
      setTooltipContent={setTooltipContent}
    />
  );
}

interface Formatter {
  format(samples: number, sampleRate: number): string;
}

export function formatDouble(
  {
    formatter,
    sampleRate,
    totalLeft,
    leftTicks,
    totalRight,
    rightTicks,
    title,
    units,
  }: {
    formatter: Formatter;
    sampleRate: number;
    totalLeft: number;
    leftTicks: number;
    totalRight: number;
    rightTicks: number;
    title: string;
    units: Units;
  },
  palette: FlamegraphPalette = DefaultPalette
): {
  tooltipData: TooltipData[];
  title: {
    text: string;
    diff: {
      text: string;
      color: string;
    };
  };
} {
  const leftRatio = totalLeft / leftTicks;
  const rightRatio = totalRight / rightTicks;

  const leftPercent = ratioToPercent(leftRatio);
  const rightPercent = ratioToPercent(rightRatio);

  const newLeft: TooltipData = {
    percent: `${leftPercent}%`,
    samples: numberWithCommas(totalLeft),
    units,
    formattedValue: formatter.format(totalLeft, sampleRate),
    tooltipType: 'flamegraph',
  };

  const newRight: TooltipData = {
    percent: `${rightPercent}%`,
    samples: numberWithCommas(totalRight),
    units,
    formattedValue: formatter.format(totalRight, sampleRate),
    tooltipType: 'flamegraph',
  };

  const totalDiff = diffPercent(leftPercent, rightPercent);

  let tooltipDiffColor = '';
  if (totalDiff > 0) {
    tooltipDiffColor = palette.badColor.rgb().string();
  } else if (totalDiff < 0) {
    tooltipDiffColor = palette.goodColor.rgb().string();
  }

  let tooltipDiffText = '';
  if (!totalLeft) {
    // this is a new function
    tooltipDiffText = '(new)';
  } else if (!totalRight) {
    // this function has been removed
    tooltipDiffText = '(removed)';
  } else if (totalDiff > 0) {
    tooltipDiffText = `(+${totalDiff.toFixed(2)}%)`;
  } else if (totalDiff < 0) {
    tooltipDiffText = `(${totalDiff.toFixed(2)}%)`;
  }

  return {
    title: {
      text: title,
      diff: {
        text: tooltipDiffText,
        color: tooltipDiffColor,
      },
    },
    tooltipData: [newLeft, newRight],
  };
}
