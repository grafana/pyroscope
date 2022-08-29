import React, { useState, useRef, useEffect, RefObject } from 'react';
import Color from 'color';
import cl from 'classnames';

import styles from './Heatmap.module.scss';

// /api/exemplars/{profile_id} mock reponse
const START_TIME = ''; // x start (unix)
const END_TIME = ''; // x end (unix)
const MIN_VALUE = 0; // min heatmap value (for color) should be white color
const MAX_VALUE = 100; // max heatmap value (for color) should be the darkest color
const HEATMAP_HEIGHT = 250;
const VALUE_BUCKETS = 20;
const TIME_BUCKETS = 120;
const COLUMNS = Array(VALUE_BUCKETS)
  .fill(Array(TIME_BUCKETS).fill(1))
  .map((col, colIndex) =>
    col.map((_: number, index: number) => {
      const color = Math.random() * 255;

      return {
        fill:
          color > 200 ? Color.rgb(10, color, color).toString() : Color('white'),
        y: colIndex,
        x: index,
      };
    })
  );

type SelectedAreaCoords = Record<'x' | 'y', number> | null;
let startCoords: SelectedAreaCoords = null;
let endCoords: SelectedAreaCoords = null;

const useHeatmapSelect = (
  canvasRef: RefObject<HTMLCanvasElement>,
  heatmapWidth: number
) => {
  const [isSelecting, setSelectingStatus] = useState(false);

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

  const drawSelectionAreaRect = (e: MouseEvent) => {
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

  useEffect(() => {
    const canvas = document.getElementById('selectionCanvas');
    if (canvas) {
      canvas.addEventListener('mousedown', startDrawing);
      canvas.addEventListener('mousemove', drawSelectionAreaRect);
      canvas.addEventListener('mouseup', endDrawing);
      canvas.addEventListener('mouseout', endDrawing); // ?
    }

    return () => {
      canvas?.removeEventListener('mousedown', startDrawing);
      canvas?.removeEventListener('mousemove', drawSelectionAreaRect);
      canvas?.addEventListener('mouseup', endDrawing);
      canvas?.addEventListener('mouseout', endDrawing); // ?
    };
  }, [startDrawing, drawSelectionAreaRect, endDrawing]);

  useEffect(() => {
    if (startCoords && endCoords) {
      const cellW = heatmapWidth / TIME_BUCKETS;
      const cellH = HEATMAP_HEIGHT / VALUE_BUCKETS;

      // if selection area hits element - it's starting point
      const matrixCoordinates = {
        xStart: Math.trunc(startCoords.x / cellW),
        yStart: Math.trunc(startCoords.y / cellH),
        xEnd: Math.trunc(endCoords.x / cellW),
        yEnd: Math.trunc(endCoords.y / cellH),
      };
    }

    // todo: redraw selected area after resize
    // useEffect(() => {
    // }, []);

    // todo escape heatmapWidth dep to redraw selected area after resize
  }, [startCoords, endCoords, heatmapWidth]);
};

function HeatMap() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [heatmapWidth, setHeatmapWith] = useState(0);

  useHeatmapSelect(canvasRef, heatmapWidth);

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
        {column.map(({ fill, x, y }: any, cellIndex: number) => (
          <rect
            data-x={x}
            data-y={y}
            // todo: add bucket click handler
            onClick={() => console.log(x, y)}
            fill={fill}
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
    <div ref={heatmapRef} className={styles.heatmapContainer}>
      y coordinates goes from top to bottom
      <br />
      <XAxis />
      <svg className={styles.heatmapSvg} height={HEATMAP_HEIGHT}>
        {generateHeatmapGrid()}
        <foreignObject height={HEATMAP_HEIGHT}>
          <canvas
            id="selectionCanvas"
            ref={canvasRef}
            height={HEATMAP_HEIGHT}
          />
        </foreignObject>
      </svg>
      <YAxis />
    </div>
  );
}

const getTicks = (max: number, min: number, ticksCount = 5) => {
  const step = (max - min) / ticksCount;
  let ticksArray = [min];

  for (let i = 1; i <= ticksCount; i++) {
    ticksArray.push(min + step * i);
  }

  return ticksArray;
};

function XAxis() {
  const ticks = getTicks(MAX_VALUE, MIN_VALUE);

  return (
    <div className={styles.xAxis} style={{ height: HEATMAP_HEIGHT + 3 }}>
      {ticks.map((tick) => (
        <div className={styles.tickContainer} key={tick}>
          <div className={styles.xTick}></div>
          <span className={cl(styles.tickValue, styles.xTickValue)}>
            {tick.toFixed(0)}
          </span>
        </div>
      ))}
    </div>
  );
}

function YAxis() {
  const ticks = getTicks(START_TIME, END_TIME, 7);

  return (
    <div className={styles.yAxis}>
      {ticks.map((tick) => (
        <div className={styles.tickContainer} key={tick}>
          <div className={styles.yTick}></div>
          <span className={cl(styles.tickValue, styles.yTickValue)}>
            {/* units ? */}
            {new Date(tick).getSeconds()}
          </span>
        </div>
      ))}
    </div>
  );
}

export default HeatMap;
