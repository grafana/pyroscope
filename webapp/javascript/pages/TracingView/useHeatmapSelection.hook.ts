import { useState, useEffect, RefObject } from 'react';

let startCoords: SelectedAreaCoordsType = null;
let endCoords: SelectedAreaCoordsType = null;
let isSelecting = false;
let selectedAreaToHeatmapRatio = 1;
export const SELECTED_AREA_BACKGROUND = 'rgb(255, 255, 0, 0.5)';
export const SELECTED_AREA_BORDER = 'rgba(250, 208, 44, 0.5)';
const DEFAULT_SELECTED_COORDINATES = { start: null, end: null };

export type SelectedAreaCoordsType = Record<'x' | 'y', number> | null;
interface SelectedCoordinates {
  start: SelectedAreaCoordsType;
  end: SelectedAreaCoordsType;
}
interface UseHeatmapSelectionProps {
  canvasRef: RefObject<HTMLCanvasElement>;
  heatmapW: number;
  heatmapH: number;
  timeBuckets: number;
  valueBuckets: number;
  // columns: number[][], add color detection to this file
  columns: number[][];
}
interface UseHeatmapSelection {
  selectedCoordinates: SelectedCoordinates;
  hasSelectedArea: boolean;
  selectedAreaToHeatmapRatio: number;
  resetSelection: () => void;
}

// maybe can remove isSelecting, hasSelectedArea
// and make coord state trigger actions
export const useHeatmapSelection = ({
  canvasRef,
  heatmapW,
  heatmapH,
  timeBuckets,
  valueBuckets,
  columns,
}: UseHeatmapSelectionProps): UseHeatmapSelection => {
  const [hasSelectedArea, setHasSelectedArea] = useState(false);
  const [selectedCoordinates, setSelectedCoordinates] =
    useState<SelectedCoordinates>(DEFAULT_SELECTED_COORDINATES);

  const resetSelection = () => {
    setHasSelectedArea(false);
    setSelectedCoordinates(DEFAULT_SELECTED_COORDINATES);
    startCoords = null;
    endCoords = null;
  };

  const startDrawing = (e: MouseEvent) => {
    const canvas = canvasRef.current as HTMLCanvasElement;
    const { left, top } = canvas.getBoundingClientRect();
    setHasSelectedArea(false);
    setSelectedCoordinates({ start: null, end: null });

    isSelecting = true;
    document.body.style.userSelect = 'none';
    startCoords = { x: e.clientX - left, y: e.clientY - top };
    endCoords = null;
  };

  const endDrawing = (e: MouseEvent) => {
    document.body.style.userSelect = 'initial';

    if (isSelecting && startCoords) {
      const canvas = canvasRef.current as HTMLCanvasElement;
      const { left, top, width, height } = canvas.getBoundingClientRect();
      setHasSelectedArea(true);
      clearRect(canvas);

      const xCursorPosition = e.clientX - left;
      const yCursorPosition = e.clientY - top;
      let xEnd;
      let yEnd;

      if (xCursorPosition < 0) {
        xEnd = 0;
      } else if (xCursorPosition > width) {
        xEnd = width;
      } else {
        xEnd = xCursorPosition;
      }

      if (yCursorPosition < 0) {
        yEnd = 0;
      } else if (yCursorPosition > height) {
        yEnd = parseInt(height.toFixed(0));
      } else {
        yEnd = yCursorPosition;
      }

      isSelecting = false;
      endCoords = { x: xEnd, y: yEnd };

      const selectedAreaW = xEnd - startCoords.x;

      if (selectedAreaW) {
        selectedAreaToHeatmapRatio = Math.abs(width / (xEnd - startCoords.x));
      } else {
        selectedAreaToHeatmapRatio = 1;
      }
    }
  };

  const handleDrawingEvent = (e: MouseEvent) => {
    const canvas = canvasRef.current as HTMLCanvasElement;

    if (isSelecting && canvas && startCoords) {
      const { left, top } = canvas.getBoundingClientRect();

      /**
       * Cursor coordinates inside canvas
       * @cursorXCoordinate - e.clientX - left
       * @cursorYCoordinate - e.clientY - top
       */
      const width = e.clientX - left - startCoords.x;
      const h = e.clientY - top - startCoords.y;

      drawRect(canvas, startCoords.x, startCoords.y, width, h);
    }
  };

  useEffect(() => {
    if (canvasRef.current) {
      canvasRef.current.addEventListener('mousedown', startDrawing);
      window.addEventListener('mousemove', handleDrawingEvent);
      window.addEventListener('mouseup', endDrawing);
    }

    return () => {
      if (canvasRef.current) {
        canvasRef.current.removeEventListener('mousedown', startDrawing);
        window.removeEventListener('mousemove', handleDrawingEvent);
        window.addEventListener('mouseup', endDrawing);
      }
    };
  }, []);

  useEffect(() => {
    const isClickEvent =
      startCoords?.x == endCoords?.x && startCoords?.y == endCoords?.y;
    const cellW = heatmapW / timeBuckets;
    const cellH = heatmapH / valueBuckets;

    if (startCoords && endCoords && !isClickEvent) {
      setSelectedCoordinates({
        start: { x: startCoords.x, y: startCoords.y },
        end: { x: endCoords.x, y: endCoords.y },
      });
      const matrixCoordinates = {
        xStart: Math.trunc(startCoords.x / cellW),
        yStart: Math.trunc(startCoords.y / cellH),
        xEnd: Math.trunc(endCoords.x / cellW),
        yEnd: Math.trunc(endCoords.y / cellH),
      };

      console.log('selected matrix coord: ', matrixCoordinates);
    }

    if (startCoords && isClickEvent) {
      clearRect(canvasRef.current as HTMLCanvasElement);
      setSelectedCoordinates({
        start: { x: startCoords.x, y: startCoords.y },
        end: null,
      });

      const x = Math.trunc(startCoords.x / cellW);
      const y = Math.trunc(startCoords.y / cellH);
      const isEmptyCell = columns[y][x] === 0;

      if (!isEmptyCell) console.log('clicked cell coord: ', { x, y });
    }
  }, [startCoords, endCoords]);

  return {
    selectedCoordinates,
    selectedAreaToHeatmapRatio,
    hasSelectedArea,
    resetSelection,
  };
};

const drawRect = (
  canvas: HTMLCanvasElement,
  x: number,
  y: number,
  w: number,
  h: number
) => {
  clearRect(canvas);
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

  ctx.fillStyle = SELECTED_AREA_BACKGROUND;
  ctx.strokeStyle = SELECTED_AREA_BORDER;
  ctx.globalAlpha = 1;
  ctx.fillRect(x, y, w, h);
  ctx.strokeRect(x, y, w, h);
};

const clearRect = (canvas: HTMLCanvasElement) => {
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
};
