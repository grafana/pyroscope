import React, { useState, useRef, useMemo, useEffect, RefObject } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';
import { interpolateViridis } from 'd3-scale-chromatic';

import { getFormatter } from '@pyroscope/legacy/flamegraph/format/format';
import type { Heatmap as HeatmapType } from '@pyroscope/services/render';
import {
  SelectedAreaCoordsType,
  useHeatmapSelection,
} from './useHeatmapSelection.hook';
import HeatmapTooltip from './HeatmapTooltip';
import { HEATMAP_HEIGHT, HEATMAP_COLORS } from './constants';
import { getTicks } from './utils';

import styles from './Heatmap.module.scss';

interface HeatmapProps {
  heatmap: HeatmapType;
  onSelection: (
    minV: number,
    maxV: number,
    startT: number,
    endT: number
  ) => void;
  sampleRate: number;
  timezone: string;
}

export function Heatmap({
  heatmap,
  onSelection,
  sampleRate,
  timezone,
}: HeatmapProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const resizedSelectedAreaRef = useRef<HTMLDivElement>(null);
  const [heatmapW, setHeatmapW] = useState(0);

  const { selectedCoordinates, selectedAreaToHeatmapRatio, resetSelection } =
    useHeatmapSelection({
      canvasRef,
      resizedSelectedAreaRef,
      heatmapW,
      heatmap,
      onSelection,
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
        sampleRate={sampleRate}
        min={heatmap.minValue}
        max={heatmap.maxValue}
        ticksCount={5}
      />
      <ResizedSelectedArea
        resizedSelectedAreaRef={resizedSelectedAreaRef}
        start={selectedCoordinates.start || { x: 0, y: 0 }}
        end={selectedCoordinates.end || { x: 0, y: 0 }}
        containerW={heatmapW}
        resizeRatio={selectedAreaToHeatmapRatio}
        handleClick={resetSelection}
      />
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
        ticksCount={7}
        timezone={timezone}
      />
      <div className={styles.legend} data-testid="color-scale">
        <span className={styles.units}>Count</span>
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
        timezone={timezone}
        sampleRate={sampleRate}
      />
    </div>
  );
}

interface ResizedSelectedAreaProps {
  resizedSelectedAreaRef: RefObject<HTMLDivElement>;
  containerW: number;
  start: SelectedAreaCoordsType;
  end: SelectedAreaCoordsType;
  resizeRatio: number;
  handleClick: () => void;
}

function ResizedSelectedArea({
  resizedSelectedAreaRef,
  containerW,
  start,
  end,
  resizeRatio,
  handleClick,
}: ResizedSelectedAreaProps) {
  const top = start.y > end.y ? end.y : start.y;
  const originalLeftOffset = start.x > end.x ? end.x : start.x;

  const w = Math.abs(containerW / resizeRatio);
  const h = Math.abs(end.y - start.y);
  const left = Math.abs((originalLeftOffset * w) / (end.x - start.x || 1));

  return (
    <>
      {h ? (
        <div
          style={{
            position: 'absolute',
            width: w,
            height: h,
            top,
            left,
            border: `1px solid ${Color.rgb(255, 149, 5).toString()}`,
          }}
        />
      ) : null}
      <div
        ref={resizedSelectedAreaRef}
        data-testid="selection-resizable-canvas"
        onClick={handleClick}
        className={styles.selectedAreaBlock}
        id="selectionArea"
        style={{
          width: w,
          height: h,
          top,
          left,
          mixBlendMode: 'overlay',
          backgroundColor: Color.rgb(255, 149, 5).toString(),
        }}
      />
    </>
  );
}

interface AxisProps {
  axis: 'x' | 'y';
  min: number;
  max: number;
  ticksCount: number;
  timezone?: string;
  sampleRate?: number;
}

function Axis({ axis, max, min, ticksCount, timezone, sampleRate }: AxisProps) {
  const yAxisformatter = sampleRate && getFormatter(max, sampleRate, 'samples');
  let ticks: string[];

  ticks = getTicks(
    min,
    max,
    { timezone, formatter: yAxisformatter, ticksCount },
    sampleRate
  );

  // There's not enough data to construct the Y axis
  if (axis === 'y' && min === 0 && max === 0) {
    ticks = ['0'];
  }

  return (
    <div
      data-testid={`${axis}-axis`}
      className={styles[`${axis}Axis`]}
      style={axis === 'y' ? { height: HEATMAP_HEIGHT } : {}}
    >
      {yAxisformatter ? (
        <div className={styles.axisUnits}>{yAxisformatter.suffix}s</div>
      ) : null}
      <div className={styles.tickValues}>
        {ticks.map((tick) => (
          <div
            role="textbox"
            className={cl(styles.tickValue, styles[`${axis}TickValue`])}
            key={tick}
          >
            <span>{tick}</span>
          </div>
        ))}
      </div>
      <div className={styles.ticksContainer}>
        {ticks.map((tick) => (
          <div className={styles.tick} key={tick} />
        ))}
      </div>
    </div>
  );
}
