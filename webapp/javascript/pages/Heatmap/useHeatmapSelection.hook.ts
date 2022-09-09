import { useState, useEffect, RefObject } from 'react';
import Color from 'color';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  fetchHeatmapSingleView,
  fetchSelectionProfile,
} from '@webapp/redux/reducers/tracing';

export const HEATMAP_HEIGHT = 250;
export const SELECTED_AREA_BACKGROUND = Color.rgb(255, 255, 0)
  .alpha(0.5)
  .toString();
const DEFAULT_SELECTED_COORDINATES = { start: null, end: null };

let startCoords: SelectedAreaCoordsType | null = null;
let endCoords: SelectedAreaCoordsType | null = null;
let isSelecting = false;
let selectedAreaToHeatmapRatio = 1;

export type SelectedAreaCoordsType = Record<'x' | 'y', number>;
interface SelectedCoordinates {
  start: SelectedAreaCoordsType | null;
  end: SelectedAreaCoordsType | null;
}
interface UseHeatmapSelectionProps {
  canvasRef: RefObject<HTMLCanvasElement>;
  heatmapW: number;
  heatmapH: number;
}
interface UseHeatmapSelection {
  selectedCoordinates: SelectedCoordinates;
  hasSelectedArea: boolean;
  selectedAreaToHeatmapRatio: number;
  resetSelection: () => void;
}

const DEFAULT_HEATMAP_PARAMS = {
  minValue: 0,
  maxValue: 1000000000,
  heatmapTimeBuckets: 128,
  heatmapValueBuckets: 32,
};

export const useHeatmapSelection = ({
  canvasRef,
  heatmapW,
  heatmapH,
}: UseHeatmapSelectionProps): UseHeatmapSelection => {
  const dispatch = useAppDispatch();
  const {
    heatmapSingleView: { heatmap: heatmapData },
  } = useAppSelector((state) => state.tracing);

  const [hasSelectedArea, setHasSelectedArea] = useState(false);
  const [selectedCoordinates, setSelectedCoordinates] =
    useState<SelectedCoordinates>(DEFAULT_SELECTED_COORDINATES);

  const { from, until, query } = useAppSelector((state) => state.continuous);
  const resetSelection = () => {
    setHasSelectedArea(false);
    setSelectedCoordinates(DEFAULT_SELECTED_COORDINATES);
    startCoords = null;
    endCoords = null;
  };

  const fetchProfile = (
    xStart: number,
    xEnd: number,
    yStart: number,
    yEnd: number
  ) => {
    if (heatmapData) {
      // reuse calculations
      const timeForPixel =
        (heatmapData.endTime - heatmapData.startTime) / heatmapW;
      const valueForPixel =
        (heatmapData.maxValue - heatmapData.minValue) / heatmapH;

      // refactor
      const smallerX = xStart > xEnd ? xEnd : xStart;
      const biggerX = xStart > xEnd ? xStart : xEnd;

      const reversedYStart = HEATMAP_HEIGHT - yStart;
      const reversedYEnd = HEATMAP_HEIGHT - yEnd;
      const smallerY =
        reversedYStart > reversedYEnd ? reversedYEnd : reversedYStart;
      const biggerY =
        reversedYStart > reversedYEnd ? reversedYStart : reversedYEnd;

      const selectionMinValue = Math.round(
        valueForPixel * smallerY + heatmapData.minValue
      );
      const selectionMaxValue = Math.round(
        valueForPixel * biggerY + heatmapData.minValue
      );
      const selectionStartTime =
        timeForPixel * smallerX + heatmapData.startTime;
      const selectionEndTime = timeForPixel * biggerX + heatmapData.startTime;

      dispatch(
        fetchSelectionProfile({
          from,
          until,
          query,
          heatmapTimeBuckets: DEFAULT_HEATMAP_PARAMS.heatmapTimeBuckets,
          heatmapValueBuckets: DEFAULT_HEATMAP_PARAMS.heatmapValueBuckets,
          selectionStartTime,
          selectionEndTime,
          selectionMinValue,
          selectionMaxValue,
        })
      );
    }
  };

  const handleCellClick = (e: MouseEvent, x: number, y: number) => {
    if (heatmapData) {
      const cellW = heatmapW / heatmapData.timeBuckets;
      const cellH = heatmapH / heatmapData.valueBuckets;
      // console.log()
      // if (startCoords && isClickEvent) {
      //   clearRect(canvasRef.current as HTMLCanvasElement);
      //   setSelectedCoordinates({
      //     start: { x: startCoords.x, y: startCoords.y },
      //     end: null,
      //   });
      //   const x = Math.trunc(startCoords.x / cellW);
      //   const y = Math.trunc(startCoords.y / cellH);
      //   const isEmptyCell = heatmapData.values[y][x] === 0;
      //   if (!isEmptyCell) console.log('clicked cell coord: ', { x, y });
      // }
      // (heatmapData.endTime - heatmapData.startTime) / heatmapW;
      // const valueForPixel =
      //   (heatmapData.maxValue - heatmapData.minValue) / heatmapH;
    }
  };

  const startDrawing = (e: MouseEvent) => {
    window.addEventListener('mousemove', handleDrawingEvent);
    window.addEventListener('mouseup', endDrawing);

    const canvas = canvasRef.current as HTMLCanvasElement;
    const { left, top } = canvas.getBoundingClientRect();
    resetSelection();

    isSelecting = true;
    document.body.style.userSelect = 'none';
    startCoords = { x: e.clientX - left, y: e.clientY - top };
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
        yEnd = parseInt(height.toFixed(0), 10);
      } else {
        yEnd = yCursorPosition;
      }

      isSelecting = false;
      endCoords = { x: xEnd, y: yEnd };

      const isClickEvent = startCoords.x === xEnd && startCoords.y === yEnd;

      if (isClickEvent) {
        handleCellClick(e, xEnd, yEnd);
      } else {
        fetchProfile(startCoords.x, xEnd, startCoords.y, yEnd);
      }

      window.removeEventListener('mousemove', handleDrawingEvent);
      window.removeEventListener('mouseup', endDrawing);

      const selectedAreaW = xEnd - startCoords.x;
      if (selectedAreaW) {
        selectedAreaToHeatmapRatio = Math.abs(width / selectedAreaW);
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
    if (from && until && query) {
      const fetchData = dispatch(
        fetchHeatmapSingleView({
          query,
          from,
          until,
          ...DEFAULT_HEATMAP_PARAMS,
        })
      );
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query]);

  useEffect(() => {
    if (canvasRef.current) {
      canvasRef.current.addEventListener('mousedown', startDrawing);
    }

    return () => {
      if (canvasRef.current) {
        canvasRef.current.removeEventListener('mousedown', startDrawing);
        window.removeEventListener('mousemove', handleDrawingEvent);
        window.removeEventListener('mouseup', endDrawing);
      }
    };
  }, [heatmapData, heatmapW]);

  useEffect(() => {
    if (heatmapData && startCoords && endCoords) {
      setSelectedCoordinates({
        start: { x: startCoords.x, y: startCoords.y },
        end: { x: endCoords.x, y: endCoords.y },
      });
    }
  }, [startCoords, endCoords, heatmapData]);

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
  ctx.globalAlpha = 1;
  ctx.fillRect(x, y, w, h);
};

const clearRect = (canvas: HTMLCanvasElement) => {
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
};
