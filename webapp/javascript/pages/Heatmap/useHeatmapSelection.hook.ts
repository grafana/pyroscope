import { useState, useEffect, RefObject } from 'react';
import Color from 'color';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { fetchHeatmapSingleView } from '@webapp/redux/reducers/tracing';

export const SELECTED_AREA_BACKGROUND = Color.rgb(255, 255, 0)
  .alpha(0.5)
  .toString();
export const SELECTED_AREA_BORDER = Color.rgb(250, 208, 44)
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

// TODO(dogfrogfog): remove when implement
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

  // to fetch initial heatmap data
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

  const changeTimeRange = (from: string, until: string) => {
    dispatch(
      fetchHeatmapSingleView({
        query,
        from,
        until,
        ...DEFAULT_HEATMAP_PARAMS,
      })
    );
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

      // console.log((heatmapData.endTime - heatmapData.startTime) / heatmapData.heatmapTimeBuckets)
      // console.log({
      //   start: { x: startCoords.x, y: startCoords.y },
      //   end: { x: endCoords.x, y: endCoords.y },
      // })
      // const matrixCoordinates = {
      //   xStart: Math.trunc(startCoords.x / cellW),
      //   yStart: Math.trunc(startCoords.y / cellH),
      //   xEnd: Math.trunc(endCoords.x / cellW),
      //   yEnd: Math.trunc(endCoords.y / cellH),
      // };

      // todo: nanoseconds -> now-smth format

      // a: from, b: until.... match with server
      changeTimeRange('now-2h', 'now-1h');

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
        window.removeEventListener('mouseup', endDrawing);
      }
    };
  }, []);

  useEffect(() => {
    const isClickEvent =
      startCoords?.x == endCoords?.x && startCoords?.y == endCoords?.y;
    const cellW = heatmapW / heatmapData.timeBuckets;
    const cellH = heatmapH / heatmapData.valueBuckets;

    if (startCoords && endCoords && !isClickEvent) {
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
  ctx.strokeStyle = SELECTED_AREA_BORDER;
  ctx.globalAlpha = 1;
  ctx.fillRect(x, y, w, h);
  ctx.strokeRect(x, y, w, h);
};

const clearRect = (canvas: HTMLCanvasElement) => {
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
};
