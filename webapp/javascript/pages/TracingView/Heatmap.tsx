import React, { useState, useRef, useEffect, useMemo } from 'react';
import cl from 'classnames';

import {
  useHeatmapSelection,
  SELECTED_AREA_BACKGROUND,
  SELECTED_AREA_BORDER,
} from './useHeatmapSelection.hook';
import * as apiResData from './mockapi';

import styles from './Heatmap.module.scss';
import Color from 'color';

const HEATMAP_HEIGHT = 250;
const CANVAS_ID = 'selectionCanvas';

function Heatmap() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [heatmapW, setHeatmapW] = useState(0);

  const {
    startTime,
    endTime,
    minDepth,
    maxDepth,
    valueBuckets,
    timeBuckets,
    columns,
  } = apiResData;

  const {
    selectedCoordinates,
    selectedAreaToHeatmapRatio,
    hasSelectedArea,
    resetSelection,
  } = useHeatmapSelection({
    canvasRef,
    heatmapW,
    heatmapH: HEATMAP_HEIGHT,
    timeBuckets,
    valueBuckets,
    columns,
  });

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
            setHeatmapW(contentBoxSize.inlineSize);
          }
        }
      });
      resizeObserver.observe(heatmapRef.current);

      return () => {
        resizeObserver.disconnect();
      };
    }
  }, []);

  const generateHeatmapGrid = useMemo(
    () =>
      columns.map((column, colIndex) => (
        <g key={colIndex}>
          {column.map((bucketItems: number, rowIndex: number) => (
            <rect
              data-x={rowIndex}
              data-y={colIndex}
              fill={
                bucketItems !== 0
                  ? getCellColor(minDepth, bucketItems)
                  : Color('white').toString()
              }
              key={rowIndex}
              x={rowIndex * (heatmapW / timeBuckets)}
              y={colIndex * (HEATMAP_HEIGHT / valueBuckets)}
              width={heatmapW / timeBuckets}
              height={HEATMAP_HEIGHT / valueBuckets}
            />
          ))}
        </g>
      )),
    [heatmapW, minDepth, maxDepth]
  );

  return (
    <div ref={heatmapRef} className={styles.heatmapContainer}>
      <XAxis minDepth={minDepth} maxDepth={maxDepth} />
      {hasSelectedArea &&
        selectedCoordinates.end &&
        selectedCoordinates.start && (
          <ResizedSelectedArea
            start={selectedCoordinates.start}
            end={selectedCoordinates.end}
            containerW={heatmapW}
            resizeRatio={selectedAreaToHeatmapRatio}
            handleClick={resetSelection}
          />
        )}
      <svg className={styles.heatmapSvg} height={HEATMAP_HEIGHT}>
        {generateHeatmapGrid}
        <foreignObject
          className={styles.selectionContainer}
          height={HEATMAP_HEIGHT}
        >
          <canvas id={CANVAS_ID} ref={canvasRef} height={HEATMAP_HEIGHT} />
        </foreignObject>
      </svg>
      <YAxis startTime={startTime} endTime={endTime} />
      <div className={styles.bucketsColors}>
        {BUCKETS_PALETTE.map((color) => (
          <div className={styles.color} style={{ backgroundColor: color }} />
        ))}
      </div>
    </div>
  );
}

interface ResizedSelectedArea {
  containerW: number;
  start: Record<'x' | 'y', number>;
  end: Record<'x' | 'y', number>;
  resizeRatio: number;
  handleClick: () => void;
}

function ResizedSelectedArea({
  containerW,
  start,
  end,
  resizeRatio,
  handleClick,
}: ResizedSelectedArea) {
  const top = start.y > end.y ? end.y : start.y;
  const originalLeftOffset = start.x > end.x ? end.x : start.x;

  const w = Math.abs(containerW / resizeRatio);
  const h = Math.abs(end.y - start.y);
  const left = Math.abs((originalLeftOffset * w) / (end.x - start.x || 1));

  return (
    <div
      onClick={handleClick}
      className={styles.selectedAreaBlock}
      style={{
        width: w,
        height: h,
        top: top,
        left,
        backgroundColor: SELECTED_AREA_BACKGROUND,
        borderColor: SELECTED_AREA_BORDER,
      }}
    />
  );
}

// maybe reuse Axis component
function XAxis({ maxDepth, minDepth }: { maxDepth: number; minDepth: number }) {
  const ticks = getTicks(maxDepth, minDepth);

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

function YAxis({ startTime, endTime }: { startTime: number; endTime: number }) {
  const ticks = getTicks(startTime, endTime, 7);

  return (
    <div className={styles.yAxis}>
      {ticks.map((tick) => (
        <div className={styles.tickContainer} key={tick}>
          <div className={styles.yTick}></div>
          <span className={cl(styles.tickValue, styles.yTickValue)}>
            {new Date(tick).getSeconds()}
          </span>
        </div>
      ))}
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

// add Color colors
const BUCKETS_PALETTE = [
  '#caf0f8',
  '#ade8f4',
  '#90e0ef',
  '#6cd5ea',
  '#48cae4',
  '#00b4d8',
  '#0096c7',
  '#0077b6',
  '#023e8a',
  '#03045e',
];

const getCellColor = (minV: number, v: number): string => {
  const colorIndex = Math.trunc((minV / v) * 10);

  return BUCKETS_PALETTE[colorIndex];
};

export default Heatmap;
