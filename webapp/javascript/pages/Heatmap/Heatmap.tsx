import React, { useState, useRef, useMemo, useEffect } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';

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
const color2 = [202, 240, 248]; // rgb(202, 240, 248)
const color1 = [3, 4, 94]; // rgb(3, 4, 94)

export function Heatmap() {
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

  // useResizeObserver doesn't work on initial render
  useEffect(() => {
    if (heatmapRef.current) {
      const { width } = heatmapRef.current.getBoundingClientRect();
      setHeatmapW(width);
    }
  }, []);

  useResizeObserver(heatmapRef.current, (entry: ResizeObserverEntry) => {
    // Firefox implements `contentBoxSize` as a single content rect, rather than an array
    const contentBoxSize = Array.isArray(entry.contentBoxSize)
      ? entry.contentBoxSize[0]
      : entry.contentBoxSize;
    (canvasRef.current as HTMLCanvasElement).width = contentBoxSize.inlineSize;
    setHeatmapW(contentBoxSize.inlineSize);
  });

  const getColor = useMemo(
    () =>
      (x: number): string => {
        const minL = Math.log10(minDepth);
        const maxL = Math.log10(maxDepth);
        const w1 = (Math.log10(x) - minL) / (maxL - minL);
        var w2 = 1 - w1;
        return Color.rgb([
          Math.round(color1[0] * w1 + color2[0] * w2),
          Math.round(color1[1] * w1 + color2[1] * w2),
          Math.round(color1[2] * w1 + color2[2] * w2),
        ]).toString();
      },
    [minDepth, maxDepth]
  );

  const generateHeatmapGrid = useMemo(
    () =>
      values.map((column, colIndex) => (
        <g role="row" key={colIndex}>
          {column.map((bucketItems: number, rowIndex: number) => (
            <rect
              role="gridcell"
              data-items={bucketItems}
              fill={
                bucketItems !== 0
                  ? getColor(bucketItems)
                  : Color('white').toString()
              }
              key={rowIndex}
              x={colIndex * (heatmapW / timeBuckets)}
              y={rowIndex * (HEATMAP_HEIGHT / valueBuckets)}
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
      <YAxis minDepth={minDepth - 1} maxDepth={maxDepth} />
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
      <svg role="img" className={styles.heatmapSvg} height={HEATMAP_HEIGHT}>
        {generateHeatmapGrid}
        <foreignObject
          className={styles.selectionContainer}
          height={HEATMAP_HEIGHT}
        >
          <canvas
            data-testid="selection-canvas"
            id={CANVAS_ID}
            ref={canvasRef}
            height={HEATMAP_HEIGHT}
          />
        </foreignObject>
      </svg>
      <XAxis startTime={startTime} endTime={endTime} />
      <div
        className={styles.bucketsColors}
        data-testid="color-scale"
        style={{
          backgroundImage: `linear-gradient(to right, ${Color.rgb(
            color2
          )} , ${Color.rgb(color1)})`,
        }}
      >
        <span role="textbox" style={{ color: Color.rgb(color1).toString() }}>
          {minDepth - 1}
        </span>
        <span role="textbox" style={{ color: Color.rgb(color2).toString() }}>
          {maxDepth}
        </span>
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
      data-testid="selection-resizable-canvas"
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

type axisFormat = 'items' | 'time';

function YAxis({ maxDepth, minDepth }: { maxDepth: number; minDepth: number }) {
  const ticks = getTicks(maxDepth, minDepth, 5, 'items');

  return (
    <div
      data-testid="y-axis"
      className={styles.yAxis}
      style={{ height: HEATMAP_HEIGHT }}
    >
      {ticks.map((tick) => (
        <div
          role="textbox"
          className={cl(styles.tick, styles.yTick)}
          key={tick}
        >
          {tick}
        </div>
      ))}
    </div>
  );
}

function XAxis({ startTime, endTime }: { startTime: number; endTime: number }) {
  const ticks = getTicks(endTime, startTime, 7, 'time');

  return (
    <div className={styles.xAxis} data-testid="x-axis">
      {ticks.map((tick) => (
        <div role="textbox" className={styles.tick} key={tick}>
          {tick}
        </div>
      ))}
    </div>
  );
}

const getTicks = (
  max: number,
  min: number,
  ticksCount: number,
  format: axisFormat
) => {
  let formatter;
  switch (format) {
    case 'time':
      formatter = (v: number) => {
        const date = new Date(v);
        return `${date.getHours()}:${date.getMinutes()}:${date.getSeconds()},${date.getMilliseconds()}`;
      };
      break;
    case 'items':
      formatter = (v: number) =>
        v > 1000 ? (v / 1000).toFixed(1) + 'k' : v.toFixed(0);
      break;
  }

  const step = (max - min) / ticksCount;
  let ticksArray = [formatter(min)];

  for (let i = 1; i <= ticksCount; i++) {
    ticksArray.push(formatter(min + step * i));
  }

  return ticksArray;
};
