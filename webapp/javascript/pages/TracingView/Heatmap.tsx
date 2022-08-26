import React, { useState, useRef, useEffect } from 'react';
// import Color from 'color';
// import globalTemperatureJSON from './global-temperature.json';

import './heatmap.scss';

// /api/exemplars/{profile_id} reponse
const START_TIME = ''; // x start (unix)
const END_TIME = ''; // x end (unix)
const MIN_VALUE = 0; // min heatmap value (for color) should be white color
const MAX_VALUE = 100; // max heatmap value (for color) should be the darkest color
const HEATMAP_HEIGHT = 250;
const VALUE_BUCKETS = 20;
const TIME_BUCKETS = 120;
const COLUMNS = (() =>
  Array(VALUE_BUCKETS)
    .fill(Array(TIME_BUCKETS).fill(1))
    .map((col, colIndex) =>
      col.map((_: number, index: number) =>
        (index + colIndex) % 2 == 0 ? 'purple' : 'white'
      )
    ))();

type SelectedAreaCoords = Record<'x' | 'y', number> | null;
let startCoords: SelectedAreaCoords = null;
let endCoords: SelectedAreaCoords = null;

function HeatMap() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [isSelecting, setSelectingStatus] = useState(false);
  const [heatmapWidth, setHeatmapWith] = useState(0);

  const startDrawing = (e: MouseEvent) => {
    if (canvasRef.current) {
      const canvas = canvasRef.current;
      const { left, top } = canvas.getBoundingClientRect();
      setSelectingStatus(true);
      startCoords = { x: e.clientX - left, y: e.clientY - top };
    }
  };

  const endDrawing = (e: MouseEvent) => {
    if (canvasRef.current) {
      const canvas = canvasRef.current;
      const { left, top } = canvas.getBoundingClientRect();
      setSelectingStatus(false);
      endCoords = { x: e.clientX - left, y: e.clientY - top };
    }
  };

  const drawRect = (e: MouseEvent) => {
    if (isSelecting && canvasRef.current && startCoords) {
      const canvas = canvasRef.current;
      const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;
      const { left, top } = canvas.getBoundingClientRect();

      ctx.clearRect(0, 0, canvas.width, canvas.height);
      ctx.fillStyle = '#FF0';
      ctx.strokeStyle = '#FAD02C';
      ctx.globalAlpha = 0.5;

      // e.clientX - left/e.clientY - top - coordinates of cursor inside canvas
      const width = e.clientX - left - startCoords.x;
      const h = e.clientY - top - startCoords.y;

      ctx.fillRect(startCoords.x, startCoords.y, width, h);
      ctx.strokeRect(startCoords.x, startCoords.y, width, h);
    }
  };

  // add events to draw selected area
  useEffect(() => {
    const canvas = document.getElementById('selectionCanvas');
    if (canvas) {
      canvas.addEventListener('mousedown', startDrawing);
      canvas.addEventListener('mousemove', drawRect);
      canvas.addEventListener('mouseup', endDrawing);
      canvas.addEventListener('mouseout', endDrawing);
    }

    return () => {
      canvas?.removeEventListener('mousedown', startDrawing);
      canvas?.removeEventListener('mousemove', drawRect);
      canvas?.addEventListener('mouseup', endDrawing);
      canvas?.addEventListener('mouseout', endDrawing);
    };
  }, [startDrawing, drawRect, endDrawing]);

  // to resize canvas
  useEffect(() => {
    if (heatmapRef.current) {
      const resizeObserver = new ResizeObserver((entries) => {
        for (const entry of entries) {
          if (entry.contentBoxSize) {
            // Firefox implements `contentBoxSize` as a single content rect, rather than an array
            const contentBoxSize = Array.isArray(entry.contentBoxSize)
              ? entry.contentBoxSize[0]
              : entry.contentBoxSize;

            (canvasRef.current as HTMLCanvasElement).width =
              contentBoxSize.inlineSize;
            setHeatmapWith(contentBoxSize.inlineSize);
          }
        }
      });
      resizeObserver.observe(heatmapRef.current);

      return () => {
        resizeObserver.disconnect();
      };
    }
  }, []);

  const generateHeatmapGrid = () =>
    COLUMNS.map((column, colIndex) => (
      <g key={colIndex}>
        {column.map((color: string, cellIndex: number) => (
          <rect
            fill={color}
            key={cellIndex}
            x={cellIndex * (heatmapWidth / TIME_BUCKETS)}
            y={colIndex * (HEATMAP_HEIGHT / VALUE_BUCKETS)}
            width={heatmapWidth / TIME_BUCKETS}
            height={HEATMAP_HEIGHT / VALUE_BUCKETS}
          />
        ))}
      </g>
    ));

  return (
    <div ref={heatmapRef} className="heatmap-container">
      <svg className="heatmap-svg" height={HEATMAP_HEIGHT}>
        {generateHeatmapGrid()}
        <foreignObject height={HEATMAP_HEIGHT}>
          <canvas
            id="selectionCanvas"
            ref={canvasRef}
            height={HEATMAP_HEIGHT}
          />
        </foreignObject>
      </svg>
    </div>
  );
}

export default HeatMap;
