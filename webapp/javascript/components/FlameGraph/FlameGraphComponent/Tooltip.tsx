import React from 'react';
import { percentDiff } from './format';
import { numberWithCommas, getFormatter } from '../../../util/format';
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

  // TODO we have an enum somewhere
  units: string;
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
  // TODO cache this to not have to instantiate all the time?
  const formatter = getFormatter(numTicks, sampleRate, units);

  const onMouseMove = (e: MouseEvent) => {
    if (!isWithinBounds(e.offsetX, e.offsetY)) {
      onMouseOut();
      return;
    }

    const data = xyToData(format, e.offsetX, e.offsetY);

    // makes it so that tooltip is always visible even if mouse is close to the right edge
    const left = Math.min(
      e.clientX + 12,
      window.innerWidth - tooltipEl.current.clientWidth - 20
    );
    const top = e.clientY + 20;

    setStyle({
      top,
      left,
      visibility: 'visible',
    });

    // format is either single, double or something else
    switch (data.format) {
      case 'single': {
        const d = formatSingle(
          formatter,
          data.percent,
          data.numBarTicks,
          props.sampleRate
        );

        setContent({
          title: {
            text: data.title,
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
        const d = formatDouble({
          formatter,
          sampleRate: props.sampleRate,
          totalLeft: data.left,
          totalRight: data.right,
          leftPercent: data.leftPercent,
          rightPercent: data.rightPercent,
          title: data.title,
        });

        setContent({
          title: d.title,
          left: d.left,
          right: d.right,
        });
        break;
      }

      default:
        throw new Error(`Unsupported format: '${JSON.stringify(data)}'`);
    }
  };

  const onMouseOut = () => {
    setStyle({
      visibility: 'hidden',
    });
  };

  React.useEffect(() => {
    // use closure to "cache" the current canvas reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const canvasEl = canvasRef.current;
    if (!canvasEl) {
      return () => {};
    }

    // watch for mouse events on the bar
    canvasEl.addEventListener('mousemove', onMouseMove);
    canvasEl.addEventListener('mouseout', onMouseOut);

    return () => {
      canvasEl.removeEventListener('mousemove', onMouseMove);
      canvasEl.removeEventListener('mouseout', onMouseOut);
    };
  }, [canvasRef.current]);

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

  const totalDiff = percentDiff(leftPercent, rightPercent).toFixed(2);

  let tooltipDiffColor = '';
  if (totalDiff > 0) {
    tooltipDiffColor = diffColorRed;
  } else if (totalDiff < 0) {
    tooltipDiffColor = diffColorGreen;
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
    tooltipDiffText = `(+${totalDiff}%)`;
  } else if (totalDiff < 0) {
    tooltipDiffText = `(${totalDiff}%)`;
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
