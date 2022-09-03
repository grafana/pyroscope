import React, { useState, useRef, useMemo, useEffect } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';

import { request } from '../../services/base';

import {
  useHeatmapSelection,
  SELECTED_AREA_BACKGROUND,
  SELECTED_AREA_BORDER,
  SelectedAreaCoordsType,
} from './useHeatmapSelection.hook';
import { exemplarsQueryHeatmap } from '../../services/exemplarsTestData';

import styles from './Heatmap.module.scss';

const HEATMAP_HEIGHT = 250;
const CANVAS_ID = 'selectionCanvas';
const BUCKETS_PALETTE = [
  Color.rgb(202, 240, 248).toString(),
  Color.rgb(173, 232, 244).toString(),
  Color.rgb(144, 224, 239).toString(),
  Color.rgb(108, 213, 234).toString(),
  Color.rgb(72, 202, 228).toString(),
  Color.rgb(0, 180, 216).toString(),
  Color.rgb(0, 150, 199).toString(),
  Color.rgb(0, 119, 182).toString(),
  Color.rgb(2, 62, 138).toString(),
  Color.rgb(3, 4, 94).toString(),
];

export function Heatmap() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [heatmapW, setHeatmapW] = useState(0);

  useEffect(() => {
    const fe = async () => {
      const data = await request('api/exemplars:query?query=app{}');

      console.log(data);
    };

    fe();
  }, []);

  const {
    startTime,
    endTime,
    minDepth,
    maxDepth,
    valueBuckets,
    timeBuckets,
    values,
  } = exemplarsQueryHeatmap;

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
    values,
  });

  useResizeObserver(heatmapRef.current, (entry: ResizeObserverEntry) => {
    // Firefox implements `contentBoxSize` as a single content rect, rather than an array
    const contentBoxSize = Array.isArray(entry.contentBoxSize)
      ? entry.contentBoxSize[0]
      : entry.contentBoxSize;
    (canvasRef.current as HTMLCanvasElement).width = contentBoxSize.inlineSize;
    setHeatmapW(contentBoxSize.inlineSize);
  });

  const generateHeatmapGrid = useMemo(
    () =>
      values.map((column, colIndex) => (
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
    <div
      ref={heatmapRef}
      className={styles.heatmapContainer}
      data-testid="heatmap-container"
    >
      <YAxis minDepth={minDepth} maxDepth={maxDepth} />
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
      <XAxis startTime={startTime} endTime={endTime} />
      <div className={styles.bucketsColors}>
        {BUCKETS_PALETTE.map((color) => (
          <div
            key={color}
            className={styles.color}
            style={{ backgroundColor: color }}
          />
        ))}
      </div>
    </div>
  );
}

interface ResizedSelectedArea {
  containerW: number;
  start: SelectedAreaCoordsType;
  end: SelectedAreaCoordsType;
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

function YAxis({ maxDepth, minDepth }: { maxDepth: number; minDepth: number }) {
  const ticks = getTicks(maxDepth, minDepth);

  return (
    <div className={styles.yAxis} style={{ height: HEATMAP_HEIGHT }}>
      {ticks.map((tick) => (
        <div className={cl(styles.tick, styles.yTick)} key={tick}>
          {tick.toFixed(0)}
        </div>
      ))}
    </div>
  );
}

function XAxis({ startTime, endTime }: { startTime: number; endTime: number }) {
  const ticks = getTicks(startTime, endTime, 7);

  return (
    <div className={styles.xAxis}>
      {ticks.map((tick) => (
        <div className={styles.tick} key={tick}>
          "1"
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

const getCellColor = (minV: number, v: number): string => {
  const colorIndex = Math.trunc((minV / v) * 10);

  return BUCKETS_PALETTE[colorIndex];
};
