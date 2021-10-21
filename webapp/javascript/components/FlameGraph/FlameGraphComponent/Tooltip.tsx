import React from 'react';
import {
  getFormatter,
  Units,
  numberWithCommas,
  formatPercent,
  ratioToPercent,
} from '@utils/format';
import { diffColorRed, diffColorGreen } from './color';

type xyToDataSingle = (
  format: 'single',
  x: number,
  y: number
) => { format: 'single'; name: string; total: number };

type xyToDataDouble = (
  format: 'double',
  x: number,
  y: number
) => {
  format: 'double';
  name: string;
  totalLeft: number;
  totalRight: number;
  barTotal: number;
};

export type TooltipProps = {
  canvasRef: React.RefObject<HTMLCanvasElement>;

  //  xyToData: xyToData;
  isWithinBounds: (x: number, y: number) => boolean;

  units: Units;
  sampleRate: number;
  numTicks: number;
} & (
  | { format: 'single'; xyToData: xyToDataSingle }
  | {
      format: 'double';
      leftTicks: number;
      rightTicks: number;
      xyToData: xyToDataDouble;
    }
);

export default function Tooltip(props: TooltipProps) {
  const { format, canvasRef, xyToData, isWithinBounds } = props;
  const [content, setContent] = React.useState({
    title: {
      text: '',
      diff: {
        text: '',
        color: '',
      },
    },
    left: '',
    right: '',
  });

  const [style, setStyle] = React.useState<React.CSSProperties>();
  const tooltipEl = React.useRef(null);

  const { numTicks, sampleRate, units } = props;

  const onMouseOut = () => {
    setStyle({
      visibility: 'hidden',
    });
  };

  // recreate the callback when the dependency changes
  // that's to evict stale props
  const memoizedOnMouseMove = React.useCallback(
    (e: MouseEvent) => {
      if (!isWithinBounds(e.offsetX, e.offsetY)) {
        onMouseOut();
        return;
      }

      const formatter = getFormatter(
        props.numTicks,
        props.sampleRate,
        props.units
      );

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
      setStyle(style);

      // set the content
      switch (props.format) {
        case 'single': {
          const data = props.xyToData(props.format, e.offsetX, e.offsetY);

          const d = formatSingle(
            formatter,
            data.total,
            props.sampleRate,
            props.numTicks
          );

          setContent({
            title: {
              text: data.name,
              diff: {
                text: '',
                color: '',
              },
            },
            left: d.left,
            right: '',
          });

          break;
        }

        case 'double': {
          const data = props.xyToData('double', e.offsetX, e.offsetY);

          const d = formatDouble({
            formatter,
            sampleRate: props.sampleRate,
            totalLeft: data.totalLeft,
            leftTicks: props.leftTicks,
            totalRight: data.totalRight,
            rightTicks: props.rightTicks,
            title: data.name,
          });

          setContent({
            title: d.title,
            left: d.left,
            right: d.right,
          });

          break;
        }
        default:
          throw new Error(`Unsupported format:'`);
      }
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
      className="flamegraph-tooltip"
      style={style}
      ref={tooltipEl}
    >
      <div
        data-testid="flamegraph-tooltip-title"
        className="flamegraph-tooltip-name"
      >
        {content.title.text}
        <span
          data-testid="flamegraph-tooltip-title-diff"
          style={{ color: content.title?.diff?.color }}
        >
          {`${content.title.diff.text.length > 0 ? ' ' : ''}${
            content.title.diff.text
          }`}
        </span>
      </div>
      <div data-testid="flamegraph-tooltip-body">
        <div data-testid="flamegraph-tooltip-left">{content.left}</div>
        <div data-testid="flamegraph-tooltip-right">{content.right}</div>
      </div>
    </div>
  );
}

interface Formatter {
  format(samples: number, sampleRate: number): string;
}

function formatSingle(
  formatter: Formatter,
  total: number,
  sampleRate: number,
  numTicks: number
) {
  const percent = formatPercent(total / numTicks);
  const left = `${percent}, ${numberWithCommas(
    total
  )} samples, ${formatter.format(total, sampleRate)}`;

  return {
    left,
  };
}

function formatDouble({
  formatter,
  sampleRate,
  totalLeft,
  leftTicks,
  totalRight,
  rightTicks,
  title,
}: {
  formatter: Formatter;
  sampleRate: number;
  totalLeft: number;
  leftTicks: number;
  totalRight: number;
  rightTicks: number;
  title: string;
}) {
  const leftRatio = totalLeft / leftTicks;
  const rightRatio = totalRight / rightTicks;

  const leftPercent = ratioToPercent(leftRatio);
  const rightPercent = ratioToPercent(rightRatio);

  const left = `Left: ${numberWithCommas(
    totalLeft
  )} samples, ${formatter.format(totalLeft, sampleRate)} (${leftPercent}%)`;

  const right = `Right: ${numberWithCommas(
    totalRight
  )} samples, ${formatter.format(totalRight, sampleRate)} (${rightPercent}%)`;

  const totalDiff = percentDiff(leftPercent, rightPercent);

  let tooltipDiffColor = '';
  if (totalDiff > 0) {
    tooltipDiffColor = diffColorRed.rgb().string();
  } else if (totalDiff < 0) {
    tooltipDiffColor = diffColorGreen.rgb().string();
  }

  // TODO unit test this
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
    left,
    right,
  };
}

function percentDiff(leftPercent: number, rightPercent: number): number {
  // difference between 2 percents
  // https://en.wikipedia.org/wiki/Relative_change_and_difference
  return ((rightPercent - leftPercent) / leftPercent) * 100;
}
