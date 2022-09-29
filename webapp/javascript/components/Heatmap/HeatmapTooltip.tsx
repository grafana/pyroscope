import React, {
  useRef,
  useEffect,
  RefObject,
  useState,
  useCallback,
} from 'react';

import TooltipWrapper from '@webapp/components/TimelineChart/TooltipWrapper';
import type { Heatmap } from '@webapp/services/render';
import { getTimeDataByXCoord, getFormatter } from './utils';

interface HeatmapTooltipProps {
  dataSourceElRef: RefObject<HTMLCanvasElement>;
  heatmapW: number;
  heatmap: Heatmap;
}

function HeatmapTooltip({
  dataSourceElRef,
  heatmapW,
  heatmap,
}: HeatmapTooltipProps) {
  const tooltipRef = useRef<HTMLDivElement>(null);
  const [tooltipParams, setTooltipParams] = useState<
    { pageX: number; pageY: number; time: string } | undefined
  >();

  const formatter = getFormatter('time', heatmap.startTime, heatmap.endTime);

  const memoizedOnMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!tooltipRef || !tooltipRef.current) {
        throw new Error('Missing tooltipElement');
      }
      const canvas = e.target as HTMLCanvasElement;
      const { left } = canvas.getBoundingClientRect();

      const xCursorPosition = e.pageX - left;
      const time = getTimeDataByXCoord(heatmap, heatmapW, xCursorPosition);

      // to fix tooltip on window edge
      const maxPageX = window.innerWidth - 130;

      setTooltipParams({
        pageX: e.pageX < maxPageX ? e.pageX - 10 : maxPageX,
        pageY: e.pageY + 10,
        time: formatter(time).toString(),
      });
    },
    [tooltipRef, setTooltipParams, heatmapW, heatmap]
  );

  // to show tooltip when move mouse over selected area
  const handleWindowMouseMove = (e: MouseEvent) => {
    if (
      (e.target as HTMLCanvasElement).id !== 'selectionCanvas' &&
      (e.target as HTMLCanvasElement).id !== 'selectionArea'
    ) {
      window.removeEventListener('mousemove', memoizedOnMouseMove);
      setTooltipParams(undefined);
    } else {
      memoizedOnMouseMove(e);
    }
  };

  const handleMouseEnter = () => {
    window.addEventListener('mousemove', handleWindowMouseMove);
  };

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
  }, [dataSourceElRef.current, memoizedOnMouseMove]);

  return (
    <div data-testid="heatmap-tooltip" ref={tooltipRef}>
      {tooltipParams && (
        <TooltipWrapper
          align="right"
          pageX={tooltipParams.pageX}
          pageY={tooltipParams.pageY}
        >
          {tooltipParams.time}
        </TooltipWrapper>
      )}
    </div>
  );
}

export default HeatmapTooltip;
