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
import type { Units } from '@pyroscope/legacy/models';

import RightClickIcon from './RightClickIcon';
import LeftClickIcon from './LeftClickIcon';

import styles from './Tooltip.module.scss';

export type TooltipData = {
  units: Units;
  percent?: string | number;
  samples?: string;
  formattedValue?: string;
  self?: string;
  total?: string;
  tooltipType: 'table' | 'flamegraph';
};

export interface TooltipProps {
  // canvas or table body ref
  dataSourceRef: RefObject<HTMLCanvasElement | HTMLTableSectionElement>;

  shouldShowFooter?: boolean;
  shouldShowTitle?: boolean;
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
}

export function Tooltip({
  shouldShowFooter = true,
  shouldShowTitle = true,
  dataSourceRef,
  clickInfoSide,
  setTooltipContent,
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
    setContent({
      title: {
        text: '',
        diff: {
          text: '',
          color: '',
        },
      },
      tooltipData: [],
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
    const dataSourceEl = dataSourceRef.current;

    // use closure to "cache" the current dataSourceRef(canvas/table) reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    if (!dataSourceEl) {
      return () => {};
    }

    // watch for mouse events on the bar
    dataSourceEl.addEventListener(
      'mousemove',
      memoizedOnMouseMove as EventListener
    );
    dataSourceEl.addEventListener('mouseout', onMouseOut);

    return () => {
      dataSourceEl.removeEventListener(
        'mousemove',
        memoizedOnMouseMove as EventListener
      );
      dataSourceEl.removeEventListener('mouseout', onMouseOut);
    };
  }, [memoizedOnMouseMove, dataSourceRef]);

  return (
    <div
      data-testid="tooltip"
      className={clsx(styles.tooltip, {
        [styles.flamegraphDiffTooltip]: content.tooltipData.length > 1,
      })}
      style={style}
      ref={tooltipRef}
    >
      {content.tooltipData.length > 0 && (
        <>
          {shouldShowTitle && (
            <div className={styles.tooltipName} data-testid="tooltip-title">
              {content.title.text}
            </div>
          )}
          <div
            className={styles.functionName}
            data-testid="tooltip-function-name"
          >
            {content.title.text}
          </div>
          {content.title.diff.text.length > 0 ? (
            <TooltipTable
              data={content.tooltipData}
              diff={content.title.diff}
            />
          ) : (
            <TooltipTable data={content.tooltipData} />
          )}
          {shouldShowFooter && <TooltipFooter clickInfoSide={clickInfoSide} />}
        </>
      )}
    </div>
  );
}

const tooltipTitles: Record<
  Units,
  { percent: string; formattedValue: string; total: string }
> = {
  objects: {
    percent: '% of objects in RAM',
    formattedValue: 'Objects in RAM',
    total: '% of total RAM',
  },
  goroutines: {
    percent: '% of goroutines',
    formattedValue: 'Goroutines',
    total: '% of total goroutines',
  },
  bytes: {
    percent: '% of RAM',
    formattedValue: 'RAM',
    total: '% of total bytes',
  },
  samples: {
    percent: 'Share of CPU',
    formattedValue: 'CPU Time',
    total: '% of total CPU',
  },
  lock_nanoseconds: {
    percent: '% of Time spent',
    formattedValue: 'Time',
    total: '% of total seconds',
  },
  nanoseconds: {
    percent: '% of Time spent',
    formattedValue: 'Time',
    total: '% of total seconds',
  },
  lock_samples: {
    percent: '% of contended locks',
    formattedValue: 'Contended locks',
    total: '% of total locks',
  },
  trace_samples: {
    percent: '% of time',
    formattedValue: 'Samples',
    total: '% of total samples',
  },
  exceptions: {
    percent: '% of thrown exceptions',
    formattedValue: 'Thrown exceptions',
    total: '% of total thrown exceptions',
  },
  unknown: {
    percent: 'Percentage',
    formattedValue: 'Units',
    total: '% of total units',
  },
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

  let renderTable: () => ReactNode;

  switch (baselineData.tooltipType) {
    case 'flamegraph':
      renderTable = () => (
        <>
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
        </>
      );
      break;
    case 'table':
      renderTable = () => (
        <>
          <thead>
            <tr>
              <td />
              <td>Self ({tooltipTitles[baselineData.units].total})</td>
              <td>Total ({tooltipTitles[baselineData.units].total})</td>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>{tooltipTitles[baselineData.units].formattedValue}:</td>
              <td>{baselineData.self}</td>
              <td>{baselineData.total}</td>
            </tr>
          </tbody>
        </>
      );
      break;
    default:
      renderTable = () => null;
  }

  return (
    <table
      data-testid="tooltip-table"
      className={clsx(styles.tooltipTable, {
        [styles[`${baselineData.tooltipType}${comparisonData ? 'Diff' : ''}`]]:
          baselineData.tooltipType,
      })}
    >
      {renderTable()}
    </table>
  );
}

function TooltipFooter({
  clickInfoSide,
}: {
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
      clickInfo = <></>;
  }

  return (
    <div data-testid="tooltip-footer" className={styles.clickInfo}>
      {clickInfo}
    </div>
  );
}
