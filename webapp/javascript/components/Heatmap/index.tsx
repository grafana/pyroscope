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
import HeatmapTooltip from './HeatmapTooltip';
import {
  SELECTED_AREA_BACKGROUND,
  HEATMAP_HEIGHT,
  HEATMAP_COLORS,
} from './constants';
import { getTicks } from './utils';

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
        const colorIndex = (x - heatmap.minDepth) / heatmap.maxDepth;

        return interpolateViridis(colorIndex);
      },
    [heatmap]
  );

  const getLegendLabel = (index: number): string => {
    switch (index) {
      case 0:
        return heatmap.maxDepth.toString();
      case 3:
        return Math.round(
          (heatmap.maxDepth - heatmap.minDepth) / 2 + heatmap.minDepth
        ).toString();
      case 6:
        return heatmap.minDepth.toString();
      default:
        return '';
    }
  };

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
      <div className={styles.legend} data-testid="color-scale">
        {HEATMAP_COLORS.map((color, index) => (
          <div key={color.toString()} className={styles.colorLabelContainer}>
            {index % 3 === 0 && (
              <span role="textbox" className={styles.label}>
                {getLegendLabel(index)}
              </span>
            )}
            <div
              className={styles.color}
              style={{
                backgroundColor: color.toString(),
              }}
            />
          </div>
        ))}
      </div>
      <HeatmapTooltip
        dataSourceElRef={canvasRef}
        heatmapW={heatmapW}
        heatmap={heatmap}
      />
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
      id="selectionArea"
      style={{
        width: w,
        height: h,
        top,
        left,
        backgroundColor: SELECTED_AREA_BACKGROUND.toString(),
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
