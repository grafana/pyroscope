import React from 'react';
import { numberWithCommas, getFormatter, Units } from '../../../util/format';
import { diffColorRed, diffColorGreen } from './color';

type xyToData = (
  format: 'single' | 'double',
  x: number,
  y: number
) =>
  | {
      format: 'double';
      left: number;
      right: number;
      title: string;
      sampleRate: number;
      leftPercent: number;
      rightPercent: number;
    }
  | {
      format: 'single';
      title: string;
      numBarTicks: number;
      percent: number;
    };

export interface TooltipProps {
  format: 'single' | 'double';
  canvasRef: React.RefObject<HTMLCanvasElement>;

  xyToData: xyToData;
  isWithinBounds: (x: number, y: number) => boolean;

  units: Units;
  sampleRate: number;
  numTicks: number;
}

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

      const d = onMouseMove({
        x: e.offsetX,
        y: e.offsetY,

        clientX: e.clientX,
        clientY: e.clientY,
        windowWidth: window.innerWidth,
        tooltipWidth: tooltipEl.current.clientWidth,

        numTicks,
        sampleRate,
        units,
        format,

        xyToData,
      });

      setStyle(d.style);
      setContent(d.content);
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
  percent: number,
  numBarTicks: number,
  sampleRate: number
) {
  const left = `${percent}, ${numberWithCommas(
    numBarTicks
  )} samples, ${formatter.format(numBarTicks, sampleRate)}`;

  return {
    left,
  };
}

function formatDouble({
  formatter,
  sampleRate,
  totalLeft,
  leftPercent,
  totalRight,
  rightPercent,
  title,
}: {
  formatter: Formatter;
  sampleRate: number;
  totalLeft: number;
  leftPercent: number;
  totalRight: number;
  rightPercent: number;
  title: string;
}) {
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

interface onMouseMoveArgs {
  x: number;
  y: number;
  clientX: number;
  clientY: number;
  windowWidth: number;
  tooltipWidth: number;
  format: 'single' | 'double';

  numTicks: number;
  sampleRate: number;
  units: Units;

  xyToData: xyToData;
}

function onMouseMove(args: onMouseMoveArgs) {
  const data = args.xyToData(args.format, args.x, args.y);

  const left = Math.min(
    args.clientX + 12,
    args.windowWidth - args.tooltipWidth - 20
  );
  const top = args.clientY + 20;

  const style: React.CSSProperties = {
    top,
    left,
    visibility: 'visible',
  };

  const formatter = getFormatter(args.numTicks, args.sampleRate, args.units);

  // format is either single, double or something else
  switch (data.format) {
    case 'single': {
      const d = formatSingle(
        formatter,
        data.percent,
        data.numBarTicks,
        args.sampleRate
      );

      return {
        style,
        content: {
          title: {
            text: data.title,
            diff: {
              text: '',
              color: '',
            },
          },
          left: d.left,
          right: '',
        },
      };
    }

    case 'double': {
      const d = formatDouble({
        formatter,
        sampleRate: args.sampleRate,
        totalLeft: data.left,
        totalRight: data.right,
        leftPercent: data.leftPercent,
        rightPercent: data.rightPercent,
        title: data.title,
      });

      return {
        style,
        content: {
          title: d.title,
          left: d.left,
          right: d.right,
        },
      };
    }

    default:
      throw new Error(`Unsupported format: '${JSON.stringify(data)}'`);
  }
}
