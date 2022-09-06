import React, { useState, useRef, useMemo, useEffect } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';

import { useAppSelector } from '@webapp/redux/hooks';
import {
  useHeatmapSelection,
  SELECTED_AREA_BACKGROUND,
  SELECTED_AREA_BORDER,
  SelectedAreaCoordsType,
} from './useHeatmapSelection.hook';

import styles from './Heatmap.module.scss';

const HEATMAP_HEIGHT = 250;
const CANVAS_ID = 'selectionCanvas';
const COLOR_EMPTY = [22, 22, 22];
const COLOR_2 = [202, 240, 248];
const COLOR_1 = [3, 4, 94];

export function Heatmap() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [heatmapW, setHeatmapW] = useState(0);
  const { heatmapSingleView } = useAppSelector((state) => state.tracing);

  const {
    selectedCoordinates,
    selectedAreaToHeatmapRatio,
    hasSelectedArea,
    resetSelection,
  } = useHeatmapSelection({
    canvasRef,
    heatmapW,
    heatmapH: HEATMAP_HEIGHT,
  });

  const {
    startTime,
    endTime,
    minDepth,
    maxDepth,
    valueBuckets,
    timeBuckets,
    values,
    maxValue,
    minValue,
  } = heatmapSingleView.heatmap;

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
          Math.round(COLOR_1[0] * w1 + COLOR_2[0] * w2),
          Math.round(COLOR_1[1] * w1 + COLOR_2[1] * w2),
          Math.round(COLOR_1[2] * w1 + COLOR_2[2] * w2),
        ]).toString();
      },
    [minDepth, maxDepth]
  );

  const generateHeatmapGrid = useMemo(
    () =>
      values.map((column, colIndex) => (
        <g role="row" key={colIndex}>
          {column.map((itemsCount: number, rowIndex: number, itemsCountArr) => (
            <rect
              role="gridcell"
              data-x-axis-value=""
              data-y-axis-value=""
              data-items={itemsCount}
              fill={
                itemsCount !== 0
                  ? getColor(itemsCount)
                  : Color.rgb(COLOR_EMPTY).toString()
              }
              key={rowIndex}
              x={colIndex * (heatmapW / timeBuckets)}
              y={
                (itemsCountArr.length - 1 - rowIndex) *
                (HEATMAP_HEIGHT / valueBuckets)
              }
              width={heatmapW / timeBuckets}
              height={HEATMAP_HEIGHT / valueBuckets}
            />
          ))}
        </g>
      )),
    [heatmapW, timeBuckets, valueBuckets, values]
  );

  return (
    <div
      ref={heatmapRef}
      className={styles.heatmapContainer}
      data-testid="heatmap-container"
    >
      <YAxis minValue={minValue} maxValue={maxValue} />
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
            COLOR_2
          )} , ${Color.rgb(COLOR_1)})`,
        }}
      >
        <span role="textbox" style={{ color: Color.rgb(COLOR_1).toString() }}>
          {minDepth}
        </span>
        <span role="textbox" style={{ color: Color.rgb(COLOR_2).toString() }}>
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

function YAxis({ maxValue, minValue }: { maxValue: number; minValue: number }) {
  const ticks = getTicks(maxValue, minValue, 5, 'items');

  return (
    <div
      data-testid="y-axis"
      className={styles.yAxis}
      style={{ height: HEATMAP_HEIGHT }}
    >
      {ticks.map((tick, index) => (
        <div
          role="textbox"
          className={cl(styles.tick, styles.yTick)}
          key={tick + index}
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
      {ticks.map((tick, index) => (
        <div role="textbox" className={styles.tick} key={tick + index}>
          {tick}
        </div>
      ))}
    </div>
  );
}

const getFormatter = (format: axisFormat) => {
  let formatter;
  switch (format) {
    case 'time':
      formatter = (v: number) => {
        const date = new Date(v);
        return `${date.getHours()}:${date.getMinutes()}:${date.getSeconds()}:${date.getMilliseconds()}`;
      };
      break;
    case 'items':
      formatter = (v: number) =>
        v > 1000 ? (v / 1000).toFixed(1) + 'k' : v.toFixed(0);
      break;
  }

  return formatter;
};

const getTicks = (
  max: number,
  min: number,
  ticksCount: number,
  format: axisFormat
) => {
  let formatter = getFormatter(format);

  const step = (max - min) / ticksCount;
  let ticksArray = [formatter(min)];

  for (let i = 1; i <= ticksCount; i++) {
    ticksArray.push(formatter(min + step * i));
  }

  return ticksArray;
};
