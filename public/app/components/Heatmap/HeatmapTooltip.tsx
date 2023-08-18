import React, {
  useRef,
  useEffect,
  RefObject,
  useState,
  useCallback,
} from 'react';

import { getFormatter } from '@pyroscope/legacy/flamegraph/format/format';
import TooltipWrapper from '@pyroscope/components/TimelineChart/TooltipWrapper';
import type { Heatmap } from '@pyroscope/services/render';
import {
  getTimeDataByXCoord,
  getBucketsDurationByYCoord,
  timeFormatter,
} from './utils';
import { HEATMAP_HEIGHT } from './constants';

import styles from './HeatmapTooltip.module.scss';

interface HeatmapTooltipProps {
  dataSourceElRef: RefObject<HTMLCanvasElement>;
  heatmapW: number;
  heatmap: Heatmap;
  timezone: string;
  sampleRate: number;
}

function HeatmapTooltip({
  dataSourceElRef,
  heatmapW,
  heatmap,
  timezone,
  sampleRate,
}: HeatmapTooltipProps) {
  const tooltipRef = useRef<HTMLDivElement>(null);
  const [tooltipParams, setTooltipParams] = useState<
    | {
        pageX: number;
        pageY: number;
        time: string;
        duration: string;
        count: number;
      }
    | undefined
  >();

  const formatter = timeFormatter(heatmap.startTime, heatmap.endTime, timezone);
  const valueFormatter = getFormatter(heatmap.maxValue, sampleRate, 'samples');

  const memoizedOnMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!tooltipRef || !tooltipRef.current) {
        throw new Error('Missing tooltipElement');
      }
      const canvas = dataSourceElRef.current as HTMLCanvasElement;
      const { left, top } = canvas.getBoundingClientRect();

      const xCursorPosition = e.pageX - left;
      const yCursorPosition = e.clientY - top;
      const time = getTimeDataByXCoord(heatmap, heatmapW, xCursorPosition);
      const bucketsDuration = getBucketsDurationByYCoord(
        heatmap,
        yCursorPosition
      );
      const cellW = heatmapW / heatmap.timeBuckets;
      const cellH = HEATMAP_HEIGHT / heatmap.valueBuckets;

      const matrixCoords = [
        Math.trunc(xCursorPosition / cellW),
        Math.trunc((HEATMAP_HEIGHT - yCursorPosition) / cellH),
      ];

      // to fix tooltip on window edge
      const maxPageX = window.innerWidth - 250;

      setTooltipParams({
        pageX: e.pageX < maxPageX ? e.pageX - 10 : maxPageX,
        pageY: e.pageY + 10,
        time: formatter(time).toString(),
        duration: valueFormatter.format(bucketsDuration, sampleRate),
        count: heatmap.values[matrixCoords[0]][matrixCoords[1]],
      });
    },
    [
      formatter,
      sampleRate,
      valueFormatter,
      tooltipRef,
      setTooltipParams,
      heatmapW,
      heatmap,
      dataSourceElRef,
    ]
  );

  // to show tooltip when move mouse over selected area
  const handleWindowMouseMove = useCallback(
    (e: MouseEvent) => {
      if (
        (e.target as HTMLCanvasElement).id !== 'selectionCanvas' &&
        (e.target as HTMLCanvasElement).id !== 'selectionArea'
      ) {
        window.removeEventListener('mousemove', memoizedOnMouseMove);
        setTooltipParams(undefined);
      } else {
        memoizedOnMouseMove(e);
      }
    },
    [memoizedOnMouseMove]
  );

  const handleMouseEnter = useCallback(() => {
    window.addEventListener('mousemove', handleWindowMouseMove);
  }, [handleWindowMouseMove]);

  useEffect(() => {
    // use closure to "cache" the current dataSourceRef(canvas/table) reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const dataSourceEl = dataSourceElRef.current;
    if (!dataSourceEl) {
      return () => {};
    }

    dataSourceEl.addEventListener('mouseenter', handleMouseEnter);

    return () => {
      dataSourceEl.removeEventListener('mouseenter', handleMouseEnter);
      window.removeEventListener('mousemove', memoizedOnMouseMove);
      window.removeEventListener('mousemove', handleWindowMouseMove);
    };
  }, [
    dataSourceElRef,
    handleMouseEnter,
    handleWindowMouseMove,
    memoizedOnMouseMove,
  ]);

  return (
    <div data-testid="heatmap-tooltip" ref={tooltipRef}>
      {tooltipParams && (
        <TooltipWrapper
          className={styles.tooltipWrapper}
          align="right"
          pageX={tooltipParams.pageX}
          pageY={tooltipParams.pageY}
        >
          <p className={styles.tooltipHeader}>{tooltipParams.time}</p>
          <div className={styles.tooltipBody}>
            <div className={styles.dataRow}>
              <span>Count: </span>
              <span>{tooltipParams.count} profiles</span>
            </div>
            <div className={styles.dataRow}>
              <span>Duration: </span>
              <span>{tooltipParams.duration}</span>
            </div>
          </div>
        </TooltipWrapper>
      )}
    </div>
  );
}

export default HeatmapTooltip;
