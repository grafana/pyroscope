import React, {
  CSSProperties,
  RefObject,
  ReactNode,
  useEffect,
  useState,
  useRef,
  useCallback,
  Dispatch,
  SetStateAction,
} from 'react';
import clsx from 'clsx';
import type { Units } from '@pyroscope/models/src';

import RightClickIcon from './RightClickIcon';
import LeftClickIcon from './LeftClickIcon';

import styles from './Tooltip.module.scss';

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

interface TooltipProps {
  // canvas or table ref
  dataSourceRef: RefObject<HTMLCanvasElement | any>;

  // footer
  shouldShowFooter?: boolean;
  clickInfoSide?: 'left' | 'right';

  setTooltipContent: (
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
  ) => void;
  // for styles
  type: 'flamegraph' | 'table';
}

export function Tooltip({
  dataSourceRef,
  shouldShowFooter = true,
  clickInfoSide,
  setTooltipContent,
  type,
}: TooltipProps) {
  const tooltipRef = useRef<HTMLDivElement>(null);
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
  const [style, setStyle] = useState<CSSProperties>();

  const onMouseOut = () => {
    setStyle({
      visibility: 'hidden',
    });
  };

  const memoizedOnMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!tooltipRef || !tooltipRef.current) {
        throw new Error('Missing tooltipElement');
      }

      const left = Math.min(
        e.clientX + 12,
        window.innerWidth - tooltipRef.current.clientWidth - 20
      );
      const top = e.clientY + 20;

      const style: React.CSSProperties = {
        top,
        left,
        visibility: 'visible',
      };

      setTooltipContent(setContent, onMouseOut, e);
      setStyle(style);
    },

    // these are the dependencies from props
    // that are going to be used in onMouseMove
    [tooltipRef, setTooltipContent]
  );

  useEffect(() => {
    // use closure to "cache" the current dataSourceRef(canvas/table) reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const dataSourceEl = dataSourceRef.current;
    if (!dataSourceEl) {
      return () => {};
    }

    // watch for mouse events on the bar
    dataSourceEl.addEventListener('mousemove', memoizedOnMouseMove);
    dataSourceEl.addEventListener('mouseout', onMouseOut);

    return () => {
      dataSourceEl.removeEventListener('mousemove', memoizedOnMouseMove);
      dataSourceEl.removeEventListener('mouseout', onMouseOut);
    };
  }, [dataSourceRef.current, memoizedOnMouseMove]);

  return (
    <div
      data-testid="tooltip"
      className={clsx(styles.tooltip, {
        [styles.flamegraphDiffTooltip]: content.tooltipData.length > 1,
      })}
      style={style}
      ref={tooltipRef}
    >
      <div className={styles.tooltipName} data-testid="tooltip-title">
        {content.title.text}
      </div>
      <div className={styles.functionName} data-testid="tooltip-function-name">
        {content.title.text}
      </div>
      {content.title.diff.text.length > 0 ? (
        <TooltipTable data={content.tooltipData} diff={content.title.diff} />
      ) : (
        <TooltipTable data={content.tooltipData} />
      )}
      <TooltipFooter
        shouldShowFooter={shouldShowFooter}
        clickInfoSide={clickInfoSide}
      />
    </div>
  );
}

export type TooltipData = {
  units: Units;
  percent: string | number;
  samples: string;
  formattedValue: string;
};

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
      data-testid="tooltip-table"
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
                    data-testid="tooltip-diff"
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

function TooltipFooter({
  shouldShowFooter,
  clickInfoSide,
}: {
  shouldShowFooter: boolean;
  clickInfoSide?: 'left' | 'right';
}) {
  let clickInfo: ReactNode;

  switch (clickInfoSide) {
    case 'right':
      clickInfo = (
        <>
          <RightClickIcon />
          <span>Right click for more node viewing options</span>
        </>
      );
      break;
    case 'left':
      clickInfo = (
        <>
          <LeftClickIcon />
          <span>Click to highlight node in flamegraph</span>
        </>
      );
      break;
    default:
      clickInfo = '<TBD ?>';
  }

  return shouldShowFooter ? (
    <div className={styles.clickInfo}>{clickInfo}</div>
  ) : null;
}
