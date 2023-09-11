import { useState, useEffect, RefObject, useCallback } from 'react';

import type { Heatmap } from '@pyroscope/services/render';
import { HEATMAP_HEIGHT } from './constants';
import { clearRect, drawRect, getSelectionData } from './utils';

const DEFAULT_SELECTED_COORDINATES = { start: null, end: null };

export type SelectedAreaCoordsType = Record<'x' | 'y', number>;
interface SelectedCoordinates {
  start: SelectedAreaCoordsType | null;
  end: SelectedAreaCoordsType | null;
}
interface UseHeatmapSelectionProps {
  canvasRef: RefObject<HTMLCanvasElement>;
  resizedSelectedAreaRef: RefObject<HTMLDivElement>;
  heatmapW: number;
  heatmap: Heatmap;
  onSelection: (
    minV: number,
    maxV: number,
    startT: number,
    endT: number
  ) => void;
}
interface UseHeatmapSelection {
  selectedCoordinates: SelectedCoordinates;
  selectedAreaToHeatmapRatio: number;
  resetSelection: () => void;
}

export const useHeatmapSelection = ({
  canvasRef,
  resizedSelectedAreaRef,
  heatmapW,
  heatmap,
  onSelection,
}: UseHeatmapSelectionProps): UseHeatmapSelection => {
  const [selectedCoordinates, setSelectedCoordinates] =
    useState<SelectedCoordinates>(DEFAULT_SELECTED_COORDINATES);

  const [selectedAreaToHeatmapRatio, setSelectedAreaToHeatmapRatio] =
    useState(1);

  const resetSelection = useCallback(() => {
    setSelectedCoordinates(DEFAULT_SELECTED_COORDINATES);
  }, [setSelectedCoordinates]);

  const handleCellClick = useCallback(
    (x: number, y: number) => {
      const cellW = heatmapW / heatmap.timeBuckets;
      const cellH = HEATMAP_HEIGHT / heatmap.valueBuckets;

      const matrixCoords = [
        Math.trunc(x / cellW),
        Math.trunc((HEATMAP_HEIGHT - y) / cellH),
      ];

      if (heatmap.values[matrixCoords[0]][matrixCoords[1]] === 0) {
        return;
      }

      // set startCoords and endCoords to draw selection rectangle for single cell
      const startCoords = {
        x: (matrixCoords[0] + 1) * cellW,
        y: HEATMAP_HEIGHT - matrixCoords[1] * cellH,
      };
      const endCoords = {
        x: matrixCoords[0] * cellW,
        y: HEATMAP_HEIGHT - (matrixCoords[1] + 1) * cellH,
      };

      setSelectedCoordinates({ start: startCoords, end: endCoords });

      const {
        selectionMinValue,
        selectionMaxValue,
        selectionStartTime,
        selectionEndTime,
      } = getSelectionData(
        heatmap,
        heatmapW,
        startCoords,
        endCoords,
        startCoords.y === HEATMAP_HEIGHT
      );

      onSelection(
        selectionMinValue,
        selectionMaxValue,
        selectionStartTime,
        selectionEndTime
      );
    },
    [heatmap, heatmapW, onSelection]
  );

  const handleDrawingEvent = useCallback(
    (e: MouseEvent) => {
      const canvas = canvasRef.current as HTMLCanvasElement;
      if (canvas && selectedCoordinates.start) {
        const { left, top } = canvas.getBoundingClientRect();
        const { x, y } = selectedCoordinates.start;

        /**
         * Cursor coordinates inside canvas
         * @cursorXCoordinate - e.clientX - left
         * @cursorYCoordinate - e.clientY - top
         */
        const width = e.clientX - left - x;
        const h = e.clientY - top - y;

        drawRect(canvas, x, y, width, h);
      }
    },
    [selectedCoordinates.start, canvasRef]
  );

  const endDrawing = useCallback(
    (e: MouseEvent) => {
      if (selectedCoordinates.start) {
        const canvas = canvasRef.current as HTMLCanvasElement;
        const { left, top, width, height } = canvas.getBoundingClientRect();
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

        const end = { x: xEnd, y: yEnd };
        const { start } = selectedCoordinates;

        const isClickEvent = start.x === xEnd && start.y === end.y;

        if (isClickEvent) {
          handleCellClick(xEnd, yEnd);
        } else {
          const {
            selectionMinValue,
            selectionMaxValue,
            selectionStartTime,
            selectionEndTime,
          } = getSelectionData(heatmap, heatmapW, start, end);

          onSelection(
            selectionMinValue,
            selectionMaxValue,
            selectionStartTime,
            selectionEndTime
          );
        }

        window.removeEventListener('mousemove', handleDrawingEvent);
        window.removeEventListener('mouseup', endDrawing);

        const selectedAreaW = end.x - start.x;
        if (selectedAreaW) {
          setSelectedAreaToHeatmapRatio(Math.abs(width / selectedAreaW));
        } else {
          setSelectedAreaToHeatmapRatio(1);
        }
      }
    },
    [
      selectedCoordinates,
      canvasRef,
      handleCellClick,
      handleDrawingEvent,
      heatmap,
      heatmapW,
      onSelection,
    ]
  );

  const startDrawing = useCallback(
    (e: MouseEvent) => {
      window.addEventListener('mousemove', handleDrawingEvent);
      window.addEventListener('mouseup', endDrawing);

      const canvas = canvasRef.current as HTMLCanvasElement;
      const { left, top } = canvas.getBoundingClientRect();
      resetSelection();

      const start = { x: e.clientX - left, y: e.clientY - top };

      setSelectedCoordinates({ ...selectedCoordinates, start });

      return () => {
        // Clean up old event listeners before adding new ones
        window.removeEventListener('mousemove', handleDrawingEvent);
        window.removeEventListener('mouseup', endDrawing);
      };
    },
    [
      canvasRef,
      resetSelection,
      endDrawing,
      handleDrawingEvent,
      selectedCoordinates,
    ]
  );

  useEffect(
    () => {
      const currentCanvasRef = canvasRef.current;
      const currentResizedSelectedAreaRef = resizedSelectedAreaRef.current;

      if (currentCanvasRef) {
        currentCanvasRef.addEventListener('mousedown', startDrawing);
      }

      if (currentResizedSelectedAreaRef) {
        currentResizedSelectedAreaRef.addEventListener(
          'mousedown',
          startDrawing
        );
      }

      return () => {
        if (currentCanvasRef) {
          currentCanvasRef.removeEventListener('mousedown', startDrawing);
        }

        if (currentResizedSelectedAreaRef) {
          currentResizedSelectedAreaRef.removeEventListener(
            'mousedown',
            startDrawing
          );
        }

        window.removeEventListener('mousemove', handleDrawingEvent);
        window.removeEventListener('mouseup', endDrawing);
      };
    },
    [
      heatmap,
      heatmapW,
      canvasRef,
      endDrawing,
      handleDrawingEvent,
      resizedSelectedAreaRef,
      startDrawing,
    ] //
  );

  // // set coordinates to display resizable selection rectangle (div element)
  // useEffect(() => {
  //   if (selectedCoordinates) {
  //     setSelectedCoordinates({
  //       start: { x: startCoords.x, y: startCoords.y },
  //       end: { x: endCoords.x, y: endCoords.y },
  //     });
  //   }
  // }, [selectedCoordinates]);

  return {
    selectedCoordinates,
    selectedAreaToHeatmapRatio,
    resetSelection,
  };
};
