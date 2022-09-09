import { useState, useEffect, RefObject } from 'react';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  fetchHeatmapSingleView,
  fetchSelectionProfile,
} from '@webapp/redux/reducers/tracing';
import {
  HEATMAP_HEIGHT,
  DEFAULT_HEATMAP_PARAMS,
  SELECTED_AREA_BACKGROUND,
} from './constants';

const DEFAULT_SELECTED_COORDINATES = { start: null, end: null };
let startCoords: SelectedAreaCoordsType | null = null;
let endCoords: SelectedAreaCoordsType | null = null;
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
      const timeForPixel =
        (heatmapData.endTime - heatmapData.startTime) / heatmapW;
      const valueForPixel =
        (heatmapData.maxValue - heatmapData.minValue) / heatmapH;

      const { smaller: smallerX, bigger: biggerX } = sortCoordinates(
        xStart,
        xEnd
      );
      const { smaller: smallerY, bigger: biggerY } = sortCoordinates(
        HEATMAP_HEIGHT - yStart,
        HEATMAP_HEIGHT - yEnd
      );

      dispatch(
        fetchSelectionProfile({
          from,
          until,
          query,
          heatmapTimeBuckets: DEFAULT_HEATMAP_PARAMS.heatmapTimeBuckets,
          heatmapValueBuckets: DEFAULT_HEATMAP_PARAMS.heatmapValueBuckets,
          selectionStartTime: timeForPixel * smallerX + heatmapData.startTime,
          selectionEndTime: timeForPixel * biggerX + heatmapData.startTime,
          selectionMinValue: Math.round(
            valueForPixel * smallerY + heatmapData.minValue
          ),
          selectionMaxValue: Math.round(
            valueForPixel * biggerY + heatmapData.minValue
          ),
        })
      );
    }
  };

  const handleCellClick = (e: MouseEvent, x: number, y: number) => {
    if (heatmapData) {
      console.log(e, x, y);
      // const cellW = heatmapW / heatmapData.timeBuckets;
      // const cellH = heatmapH / heatmapData.valueBuckets;
      // const x = Math.trunc(startCoords.x / cellW);
      // const y = Math.trunc(startCoords.y / cellH);
      // console.log()
      // if (startCoords && isClickEvent) {
      //   clearRect(canvasRef.current as HTMLCanvasElement);
      //   setSelectedCoordinates({
      //     start: { x: startCoords.x, y: startCoords.y },
      //     end: null,
      //   });

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

    startCoords = { x: e.clientX - left, y: e.clientY - top };
  };

  const endDrawing = (e: MouseEvent) => {
    if (startCoords) {
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

    if (canvas && startCoords) {
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

  // set coordinates to display resizable selection rectangle (div element)
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

const sortCoordinates = (
  v1: number,
  v2: number
): { smaller: number; bigger: number } => {
  const isFirstBigger = v1 > v2;

  return {
    smaller: isFirstBigger ? v2 : v1,
    bigger: isFirstBigger ? v1 : v2,
  };
};
