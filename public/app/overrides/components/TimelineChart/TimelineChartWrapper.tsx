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
  coordsToCanvasPos?: unknown;
  canvasX?: number;
}

export type TimelineData = any;

interface TimelineChartWrapperProps {
  id: string;
  timelineA: unknown;
  timezone: string;
  annotations?: unknown;
  selectionType: unknown;
  onSelect: (from: string, until: string) => void;
  onHoverDisplayTooltip?: React.FC<TooltipCallbackProps>;
  format?: any;
  timelineB?: unknown;
  selectionWithHandler?: unknown;
  syncCrosshairsWith?: unknown;
  selection?: {
    left?: unknown;
    right?: unknown;
  };
  ContextMenu?: (props: any) => React.ReactNode;
  height?: unknown;
  title?: React.ReactNode;
}
export default function (props: TimelineChartWrapperProps) {
  const ref = useRef<HTMLDivElement>(null);

  // Horrible hack to hide the parent <Box>
  // This won't be necessary after Timelines are implemented properly
  useEffect(() => {
    const parentElement = ref.current?.parentElement?.parentElement;

    // When timelines are within a pyro-flamegraph (eg in comparison page, don't do anything)
    if (
      parentElement?.parentElement?.tagName.toLowerCase() !==
        'pyro-flamegraph' &&
      parentElement
    ) {
      parentElement.style.display = 'none';
    }
  }, [ref.current]);

  return <div ref={ref}></div>;
}
