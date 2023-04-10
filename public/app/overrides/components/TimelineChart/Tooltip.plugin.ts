// TooltipCallbackProps refers to the data available for the tooltip body construction
export interface TooltipCallbackProps {
  timeLabel: string;
  values: Array<{
    closest: number[];
    color: number[];
    // TODO: remove this
    tagName: string;
  }>;
  coordsToCanvasPos?: any;
  canvasX?: number;
}
