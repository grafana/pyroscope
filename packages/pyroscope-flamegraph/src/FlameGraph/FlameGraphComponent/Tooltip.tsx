import React from 'react';
import { Maybe } from 'true-myth';
import clsx from 'clsx';
import type { Units } from '@pyroscope/models/src';
import type { Unwrapped } from 'true-myth/maybe';
import {
  getFormatter,
  numberWithCommas,
  formatPercent,
  ratioToPercent,
} from '../../format/format';

import { DefaultPalette, FlamegraphPalette } from './colorPalette';
import styles from './Tooltip.module.scss';

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

export type TooltipProps = {
  canvasRef: React.RefObject<HTMLCanvasElement>;

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

const tooltipTitles: Record<
  Units,
  { percent: string; formattedValue: string }
> = {
  objects: {
    percent: '% of objects in RAM',
    formattedValue: 'RAM amount',
  },
  goroutines: {
    percent: '% of goroutines',
    formattedValue: 'goroutines',
  },
  bytes: {
    percent: '% of RAM',
    formattedValue: 'bytes',
  },
  samples: {
    percent: 'Share of CPU',
    formattedValue: 'CPU Time',
  },
  lock_nanoseconds: {
    percent: '% of Time spent',
    formattedValue: 'seconds',
  },
  lock_samples: {
    percent: '% of contended locks',
    formattedValue: 'locks',
  },
  trace_samples: {
    percent: '% of time',
    formattedValue: 'samples',
  },
  '': {
    percent: '',
    formattedValue: '',
  },
};

type TooltipData = {
  units: Units;
  percent: string | number;
  samples: string;
  formattedValue: string;
};

export default function Tooltip(props: TooltipProps) {
  const { format, canvasRef, xyToData } = props;
  const [content, setContent] = React.useState({
    title: {
      text: '',
      diff: {
        text: '',
        color: '',
      },
    },
    tooltipData: [] as TooltipData[],
  });

  const [style, setStyle] = React.useState<React.CSSProperties>();
  const tooltipEl = React.useRef<HTMLDivElement>(null);

  const { numTicks, sampleRate, units, leftTicks, rightTicks, palette } = props;
  const onMouseOut = () => {
    setStyle({
      visibility: 'hidden',
    });
  };

  // recreate the callback when the dependency changes
  // that's to evict stale props
  const memoizedOnMouseMove = React.useCallback(
    (e: MouseEvent) => {
      const formatter = getFormatter(numTicks, sampleRate, units);

      if (!tooltipEl || !tooltipEl.current) {
        throw new Error('Missing tooltipElement');
      }

      const left = Math.min(
        e.clientX + 12,
        window.innerWidth - tooltipEl.current.clientWidth - 20
      );
      const top = e.clientY + 20;

      const style: React.CSSProperties = {
        top,
        left,
        visibility: 'visible',
      };

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

      // set the content
      switch (data.format) {
        case 'single': {
          const newLeftContent: TooltipData = {
            percent: formatPercent(data.total / numTicks),
            samples:
              units === 'trace_samples' ? '' : numberWithCommas(data.total),
            units,
            formattedValue: formatter.format(data.total, sampleRate),
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

      setStyle(style);
    },

    // these are the dependencies from props
    // that are going to be used in onMouseMove
    [numTicks, sampleRate, units, format, xyToData]
  );

  React.useEffect(() => {
    // use closure to "cache" the current canvas reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const canvasEl = canvasRef.current;
    if (!canvasEl) {
      return () => {};
    }

    // watch for mouse events on the bar
    canvasEl.addEventListener('mousemove', memoizedOnMouseMove);
    canvasEl.addEventListener('mouseout', onMouseOut);

    return () => {
      canvasEl.removeEventListener('mousemove', memoizedOnMouseMove);
      canvasEl.removeEventListener('mouseout', onMouseOut);
    };
  }, [canvasRef.current, memoizedOnMouseMove]);

  return (
    <div
      role="tooltip"
      data-testid="flamegraph-tooltip"
      className={clsx(styles.flamegraphTooltip, {
        [styles.flamegraphDiffTooltip]: content.tooltipData.length > 1,
      })}
      style={style}
      ref={tooltipEl}
    >
      <div
        className={styles.flamegraphTooltipName}
        data-testid="flamegraph-tooltip-title"
      >
        {content.title.text}
      </div>
      <div
        className={styles.functionName}
        data-testid="flamegraph-tooltip-function-name"
      >
        {content.title.text}
      </div>

      {content.title.diff.text.length > 0 ? (
        <TooltipTable data={content.tooltipData} diff={content.title.diff} />
      ) : (
        <TooltipTable data={content.tooltipData} />
      )}
    </div>
  );
}

function TooltipTable({
  data,
  diff,
}: {
  data: TooltipData[];
  diff?: { text: string; color: string };
}) {
  const [baselineData, comparisonData] = data;

  if (!baselineData) {
    return null;
  }

  return (
    <table
      data-testid="flamegraph-tooltip-table"
      className={clsx(styles.tooltipTable, {
        [styles.tooltipDiffTable]: comparisonData,
      })}
    >
      {comparisonData && (
        <thead>
          <tr>
            <th />
            <th>Baseline</th>
            <th>Comparison</th>
            <th>Diff</th>
          </tr>
        </thead>
      )}
      <tbody>
        <tr>
          <td>{tooltipTitles[baselineData.units].percent}:</td>
          <td>{baselineData.percent}</td>
          {comparisonData && (
            <>
              <td>{comparisonData.percent}</td>
              <td>
                {diff && (
                  <span
                    data-testid="flamegraph-tooltip-diff"
                    style={{ color: diff.color }}
                  >
                    {diff.text}
                  </span>
                )}
              </td>
            </>
          )}
        </tr>
        <tr>
          <td>{tooltipTitles[baselineData.units].formattedValue}:</td>
          <td>{baselineData.formattedValue}</td>
          {comparisonData && (
            <>
              <td>{comparisonData.formattedValue}</td>
              <td />
            </>
          )}
        </tr>
        <tr>
          <td>Samples:</td>
          <td>{baselineData.samples}</td>
          {comparisonData && (
            <>
              <td>{comparisonData.samples}</td>
              <td />
            </>
          )}
        </tr>
      </tbody>
    </table>
  );
}

interface Formatter {
  format(samples: number, sampleRate: number): string;
}

function formatDouble(
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
) {
  const leftRatio = totalLeft / leftTicks;
  const rightRatio = totalRight / rightTicks;

  const leftPercent = ratioToPercent(leftRatio);
  const rightPercent = ratioToPercent(rightRatio);

  const newLeft: TooltipData = {
    percent: leftPercent + '%',
    samples: numberWithCommas(totalLeft),
    units,
    formattedValue: formatter.format(totalLeft, sampleRate),
  };

  const newRight: TooltipData = {
    percent: rightPercent + '%',
    samples: numberWithCommas(totalRight),
    units,
    formattedValue: formatter.format(totalRight, sampleRate),
  };

  const totalDiff = percentDiff(leftPercent, rightPercent);

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

function percentDiff(leftPercent: number, rightPercent: number): number {
  // difference between 2 percents
  // https://en.wikipedia.org/wiki/Relative_change_and_difference
  return ((rightPercent - leftPercent) / leftPercent) * 100;
}
