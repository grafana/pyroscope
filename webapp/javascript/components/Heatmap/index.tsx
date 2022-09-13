import React, { useState, useRef, useMemo, useEffect } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';
import { interpolateViridis } from 'd3-scale-chromatic';

import type { Heatmap } from '@webapp/services/render';
import {
  SelectedAreaCoordsType,
  useHeatmapSelection,
} from './useHeatmapSelection.hook';
import {
  SELECTED_AREA_BACKGROUND,
  HEATMAP_HEIGHT,
  VIRIDIS_COLORS,
} from './constants';

// eslint-disable-next-line css-modules/no-unused-class
import styles from './Heatmap.module.scss';

interface HeatmapProps {
  heatmap: Heatmap;
  onSelection: (
    minV: number,
    maxV: number,
    startT: number,
    endT: number
  ) => void;
}

export function Heatmap({ heatmap, onSelection }: HeatmapProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [heatmapW, setHeatmapW] = useState(0);

  const {
    selectedCoordinates,
    selectedAreaToHeatmapRatio,
    hasSelectedArea,
    resetSelection,
  } = useHeatmapSelection({ canvasRef, heatmapW, heatmap, onSelection });

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
        if (x === 0) {
          return Color.rgb(22, 22, 22).toString();
        }

        // from 0 to 1
        const colorIndex = 1 - (x - heatmap.minDepth) / heatmap.maxDepth;

        return interpolateViridis(colorIndex);
      },
    [heatmap]
  );

  const heatmapGrid = (() =>
    heatmap.values.map((column, colIndex) => (
      // eslint-disable-next-line react/no-array-index-key
      <g role="row" key={colIndex}>
        {column.map((itemsCount: number, rowIndex: number, itemsCountArr) => (
          <rect
            role="gridcell"
            data-items={itemsCount}
            fill={getColor(itemsCount)}
            // eslint-disable-next-line react/no-array-index-key
            key={rowIndex}
            x={colIndex * (heatmapW / heatmap.timeBuckets)}
            y={
              (itemsCountArr.length - 1 - rowIndex) *
              (HEATMAP_HEIGHT / heatmap.valueBuckets)
            }
            width={heatmapW / heatmap.timeBuckets}
            height={HEATMAP_HEIGHT / heatmap.valueBuckets}
          />
        ))}
      </g>
    )))();

  return (
    <div
      ref={heatmapRef}
      className={styles.heatmapContainer}
      data-testid="heatmap-container"
    >
      <Axis
        axis="y"
        min={heatmap.minValue}
        max={heatmap.maxValue}
        ticksNumber={5}
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
        {heatmapGrid}
        <foreignObject
          className={styles.selectionContainer}
          height={HEATMAP_HEIGHT}
        >
          <canvas
            data-testid="selection-canvas"
            id="selectionCanvas"
            ref={canvasRef}
            height={HEATMAP_HEIGHT}
          />
        </foreignObject>
      </svg>
      <Axis
        axis="x"
        min={heatmap.startTime}
        max={heatmap.endTime}
        ticksNumber={7}
      />
      <div
        className={styles.bucketsColors}
        data-testid="color-scale"
        style={{
          backgroundImage: `linear-gradient(to right, ${VIRIDIS_COLORS[0]} , ${VIRIDIS_COLORS[1]}, ${VIRIDIS_COLORS[2]}, ${VIRIDIS_COLORS[3]}, ${VIRIDIS_COLORS[4]})`,
        }}
      >
        <span
          role="textbox"
          style={{ color: Color.rgb(VIRIDIS_COLORS[4]).toString() }}
        >
          {heatmap.minDepth}
        </span>
        <span
          role="textbox"
          style={{ color: Color.rgb(VIRIDIS_COLORS[0]).toString() }}
        >
          {heatmap.maxDepth}
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
        top,
        left,
        backgroundColor: SELECTED_AREA_BACKGROUND,
      }}
    />
  );
}

interface AxisProps {
  axis: 'x' | 'y';
  min: number;
  max: number;
  ticksNumber: number;
}

const FORMAT_MAP = {
  x: 'time' as const,
  y: 'value' as const,
};

function Axis({ axis, max, min, ticksNumber }: AxisProps) {
  const ticks = getTicks(FORMAT_MAP[axis], min, max, ticksNumber);

  return (
    <div
      data-testid={`${axis}-axis`}
      className={styles[`${axis}Axis`]}
      style={axis === 'y' ? { height: HEATMAP_HEIGHT } : {}}
    >
      {ticks.map((tick) => (
        <div
          role="textbox"
          className={cl(styles.tick, styles[`${axis}Tick`])}
          key={tick}
        >
          {tick}
        </div>
      ))}
    </div>
  );
}

const getFormatter = (format: 'value' | 'time') => {
  let formatter;
  switch (format) {
    case 'time':
      formatter = (v: number) => {
        const date = new Date(v / 1000000);

        return date.toLocaleTimeString();
      };
      break;
    case 'value':
      formatter = (v: number) =>
        v > 1000 ? `${(v / 1000).toFixed(1)}k` : v.toFixed(0);
      break;
    default:
      formatter = (v: number) => v;
  }

  return formatter;
};

const getTicks = (
  format: 'value' | 'time',
  min: number,
  max: number,
  ticksCount: number
) => {
  const formatter = getFormatter(format);

  const step = (max - min) / ticksCount;
  const ticksArray = [formatter(min)];

  for (let i = 1; i <= ticksCount; i += 1) {
    ticksArray.push(formatter(min + step * i));
  }

  return ticksArray;
};
