import type { Heatmap } from '@webapp/services/render';

import { SELECTED_AREA_BACKGROUND, HEATMAP_HEIGHT } from './constants';
import type { SelectedAreaCoordsType } from './useHeatmapSelection.hook';

export const drawRect = (
  canvas: HTMLCanvasElement,
  x: number,
  y: number,
  w: number,
  h: number
) => {
  clearRect(canvas);
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

  ctx.fillStyle = SELECTED_AREA_BACKGROUND.toString();
  ctx.globalAlpha = 1;
  ctx.fillRect(x, y, w, h);
};

export const clearRect = (canvas: HTMLCanvasElement) => {
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
};

export const sortCoordinates = (
  v1: number,
  v2: number
): { smaller: number; bigger: number } => {
  const isFirstBigger = v1 > v2;

  return {
    smaller: isFirstBigger ? v2 : v1,
    bigger: isFirstBigger ? v1 : v2,
  };
};

interface SelectionData {
  selectionMinValue: number;
  selectionMaxValue: number;
  selectionStartTime: number;
  selectionEndTime: number;
}

// TODO(dogfrogfog): extend calculating data
export const getTimeDataByXCoord = (
  heatmap: Heatmap,
  heatmapW: number,
  x: number
) => {
  const timeForPixel = (heatmap.endTime - heatmap.startTime) / heatmapW;

  return x * timeForPixel + heatmap.startTime;
};

export const getSelectionData = (
  heatmap: Heatmap,
  heatmapW: number,
  startCoords: SelectedAreaCoordsType,
  endCoords: SelectedAreaCoordsType,
  isClickOnYBottomEdge?: boolean
): SelectionData => {
  const timeForPixel = (heatmap.endTime - heatmap.startTime) / heatmapW;
  const valueForPixel = (heatmap.maxValue - heatmap.minValue) / HEATMAP_HEIGHT;

  const { smaller: smallerX, bigger: biggerX } = sortCoordinates(
    startCoords.x,
    endCoords.x
  );
  const { smaller: smallerY, bigger: biggerY } = sortCoordinates(
    HEATMAP_HEIGHT - startCoords.y,
    HEATMAP_HEIGHT - endCoords.y
  );

  // to fetch correct profiles when clicking on edge cells
  const selectionMinValue = Math.round(
    valueForPixel * smallerY + heatmap.minValue
  );

  return {
    selectionMinValue: isClickOnYBottomEdge
      ? selectionMinValue - 1
      : selectionMinValue,
    selectionMaxValue: Math.round(valueForPixel * biggerY + heatmap.minValue),
    selectionStartTime: timeForPixel * smallerX + heatmap.startTime,
    selectionEndTime: timeForPixel * biggerX + heatmap.startTime,
  };
};

export const getFormatter = (format: 'value' | 'time') => {
  let formatter;
  switch (format) {
    case 'time':
      formatter = (v: number) => {
        const date = new Date(v / 1000000);

        return date.toLocaleTimeString();
      };
      break;
    case 'value':
      formatter = (v: number) =>
        v > 1000 ? `${(v / 1000).toFixed(1)}k` : v.toFixed(0);
      break;
    default:
      formatter = (v: number) => v;
  }

  return formatter;
};

export const getTicks = (
  format: 'value' | 'time',
  min: number,
  max: number,
  ticksCount: number
) => {
  const formatter = getFormatter(format);

  const step = (max - min) / ticksCount;
  const ticksArray = [formatter(min)];

  for (let i = 1; i <= ticksCount; i += 1) {
    ticksArray.push(formatter(min + step * i));
  }

  return ticksArray;
};
