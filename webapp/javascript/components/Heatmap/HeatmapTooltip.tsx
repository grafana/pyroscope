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
  canvasRef: RefObject<HTMLCanvasElement>;
  heatmapW: number;
  heatmap: Heatmap;
}

const formatter = getFormatter('time');

function HeatmapTooltip({ canvasRef, heatmapW, heatmap }: HeatmapTooltipProps) {
  const tooltipRef = useRef<HTMLDivElement>(null);
  const [tooltipParams, setTooltipParams] = useState<
    { pageX: number; pageY: number; time: string } | undefined
  >();

  const onMouseOut = () => setTooltipParams(undefined);

  const memoizedOnMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!tooltipRef || !tooltipRef.current) {
        throw new Error('Missing tooltipElement');
      }
      const canvas = e.target as HTMLCanvasElement;
      const { left } = canvas.getBoundingClientRect();

      const xCursorPosition = e.pageX - left;
      const time = getTimeDataByXCoord(heatmap, heatmapW, xCursorPosition);

      setTooltipParams({
        pageX: e.pageX,
        pageY: e.pageY,
        time: formatter(time).toString(),
      });
    },
    [tooltipRef, setTooltipParams, heatmapW, heatmap]
  );

  useEffect(() => {
    // use closure to "cache" the current dataSourceRef(canvas/table) reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const dataSourceEl = canvasRef.current;
    if (!dataSourceEl) {
      return () => {};
    }

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
  }, [canvasRef.current, memoizedOnMouseMove]);

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
