import React, { useState, useRef, useMemo, useEffect } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';

import { useAppSelector } from '@webapp/redux/hooks';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
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
  const {
    heatmapSingleView: { heatmap: heatmapData },
  } = useAppSelector((state) => state.tracing);

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

  useEffect(() => {
    if (heatmapRef.current) {
      const { width } = heatmapRef.current.getBoundingClientRect();
      setHeatmapW(width);
    }
  }, []);

  useResizeObserver(heatmapRef.current, (entry: ResizeObserverEntry) => {
    if (canvasRef.current) {
      // Firefox implements `contentBoxSize` as a single content rect, rather than an array
      const contentBoxSize = Array.isArray(entry.contentBoxSize)
        ? entry.contentBoxSize[0]
        : entry.contentBoxSize;

      canvasRef.current.width = contentBoxSize.inlineSize;
      setHeatmapW(contentBoxSize.inlineSize);
    }
  });

  const getColor = useMemo(
    () =>
      (x: number): string => {
        if (heatmapData) {
          const minL = Math.log10(heatmapData.minDepth);
          const maxL = Math.log10(heatmapData.maxDepth);
          const w1 = (Math.log10(x) - minL) / (maxL - minL);
          const w2 = 1 - w1;

          return Color.rgb([
            Math.round(COLOR_1[0] * w1 + COLOR_2[0] * w2),
            Math.round(COLOR_1[1] * w1 + COLOR_2[1] * w2),
            Math.round(COLOR_1[2] * w1 + COLOR_2[2] * w2),
          ]).toString();
        }

        return '';
      },
    [heatmapData]
  );

  const generateHeatmapGrid = useMemo(
    () =>
      heatmapData?.values.map((column, colIndex) => (
        // eslint-disable-next-line react/no-array-index-key
        <g role="row" key={colIndex}>
          {column?.map(
            (itemsCount: number, rowIndex: number, itemsCountArr) => (
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
                // eslint-disable-next-line react/no-array-index-key
                key={rowIndex}
                x={colIndex * (heatmapW / heatmapData.timeBuckets)}
                y={
                  (itemsCountArr.length - 1 - rowIndex) *
                  (HEATMAP_HEIGHT / heatmapData.valueBuckets)
                }
                width={heatmapW / heatmapData.timeBuckets}
                height={HEATMAP_HEIGHT / heatmapData.valueBuckets}
              />
            )
          )}
        </g>
      )),
    [heatmapW, heatmapData]
  );

  return (
    <div
      ref={heatmapRef}
      className={styles.heatmapContainer}
      data-testid="heatmap-container"
    >
      {!heatmapData ? (
        <LoadingSpinner />
      ) : (
        <>
          <YAxis
            minValue={heatmapData.minValue}
            maxValue={heatmapData.maxValue}
          />
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
          <XAxis
            startTime={heatmapData.startTime}
            endTime={heatmapData.endTime}
          />
          <div
            className={styles.bucketsColors}
            data-testid="color-scale"
            style={{
              backgroundImage: `linear-gradient(to right, ${Color.rgb(
                COLOR_2
              )} , ${Color.rgb(COLOR_1)})`,
            }}
          >
            <span
              role="textbox"
              style={{ color: Color.rgb(COLOR_1).toString() }}
            >
              {heatmapData.minDepth}
            </span>
            <span
              role="textbox"
              style={{ color: Color.rgb(COLOR_2).toString() }}
            >
              {heatmapData.maxDepth}
            </span>
          </div>
        </>
      )}
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
        top,
        left,
        backgroundColor: SELECTED_AREA_BACKGROUND,
        borderColor: SELECTED_AREA_BORDER,
      }}
    />
  );
}

type axisFormat = 'items' | 'time';

function YAxis({ maxValue, minValue }: { maxValue: number; minValue: number }) {
  const ticks = getTicks(5, 'items', maxValue, minValue);

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
  const ticks = getTicks(7, 'time', endTime, startTime);

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

const getFormatter = (format: axisFormat) => {
  let formatter;
  // eslint-disable-next-line default-case
  switch (format) {
    case 'time':
      formatter = (v: number) => {
        const date = new Date(v / 1000000);

        return date.toLocaleTimeString();
      };
      break;
    case 'items':
      formatter = (v: number) =>
        v > 1000 ? `${(v / 1000).toFixed(1)}k` : v.toFixed(0);
      break;
  }

  return formatter;
};

const getTicks = (
  ticksCount: number,
  format: axisFormat,
  max: number,
  min: number
) => {
  const formatter = getFormatter(format);

  const step = (max - min) / ticksCount;
  const ticksArray = [formatter(min)];

  for (let i = 1; i <= ticksCount; i += 1) {
    ticksArray.push(formatter(min + step * i));
  }

  return ticksArray;
};
