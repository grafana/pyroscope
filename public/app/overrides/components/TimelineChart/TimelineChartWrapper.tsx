import { useEffect, useRef } from 'react';

// TooltipCallbackProps refers to the data available for the tooltip body construction
interface TooltipCallbackProps {
  timeLabel: string;
  values: Array<{
    closest: number[];
    color: number[];
    // TODO: remove this
    tagName: string;
  }>;
  //  coordsToCanvasPos?: jquery.flot.axis['p2c'];
  coordsToCanvasPos?: unknown;
  canvasX?: number;
}

interface TimelineChartWrapperProps {
  id: string;
  timelineA: unknown;
  height: unknown;
  timezone: string;
  title: React.ReactNode;
  annotations: unknown;
  ContextMenu: unknown;
  selectionType: unknown;
  onSelect: (from: string, until: string) => void;
  onHoverDisplayTooltip?: React.FC<TooltipCallbackProps>;
}
export default function (props: TimelineChartWrapperProps) {
  const ref = useRef<HTMLDivElement>(null);

  // Since this element is inside a <Box>, also make the box hidden
  useEffect(() => {
    const parentElement = ref.current?.parentElement?.parentElement;
    if (parentElement) {
      parentElement.style.display = 'none';
    }
  }, [ref.current]);

  return <div ref={ref}></div>;
}
