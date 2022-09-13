import React, { useState, useRef, useMemo, useEffect } from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import Color from 'color';
import cl from 'classnames';

import { useAppSelector } from '@webapp/redux/hooks';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import {
  SelectedAreaCoordsType,
  useHeatmapSelection,
} from './useHeatmapSelection.hook';
import {
  SELECTED_AREA_BACKGROUND,
  HEATMAP_HEIGHT,
  COLOR_1,
  COLOR_2,
  COLOR_EMPTY,
} from './constants';

// eslint-disable-next-line css-modules/no-unused-class
import styles from './Heatmap.module.scss';

export function Heatmap() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const heatmapRef = useRef<HTMLDivElement>(null);
  const [heatmapW, setHeatmapW] = useState(0);
  const { exemplarsSingleView } = useAppSelector((state) => state.tracing);

  const {
    selectedCoordinates,
    selectedAreaToHeatmapRatio,
    hasSelectedArea,
    resetSelection,
  } = useHeatmapSelection({ canvasRef, heatmapW });

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
        if (exemplarsSingleView.heatmap) {
          const minL = Math.log10(exemplarsSingleView.heatmap.minDepth);
          const maxL = Math.log10(exemplarsSingleView.heatmap.maxDepth);
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
    [exemplarsSingleView.heatmap]
  );

  const heatmapGrid = (() => {
    switch (exemplarsSingleView.type) {
      case 'loaded':
      case 'reloading': {
        return exemplarsSingleView.heatmap.values.map((column, colIndex) => (
          // eslint-disable-next-line react/no-array-index-key
          <g role="row" key={colIndex}>
            {column.map(
              (itemsCount: number, rowIndex: number, itemsCountArr) => (
                <rect
                  role="gridcell"
                  data-items={itemsCount}
                  fill={
                    itemsCount !== 0
                      ? getColor(itemsCount)
                      : Color.rgb(COLOR_EMPTY).toString()
                  }
                  // eslint-disable-next-line react/no-array-index-key
                  key={rowIndex}
                  x={
                    colIndex *
                    (heatmapW / exemplarsSingleView.heatmap.timeBuckets)
                  }
                  y={
                    (itemsCountArr.length - 1 - rowIndex) *
                    (HEATMAP_HEIGHT / exemplarsSingleView.heatmap.valueBuckets)
                  }
                  width={heatmapW / exemplarsSingleView.heatmap.timeBuckets}
                  height={
                    HEATMAP_HEIGHT / exemplarsSingleView.heatmap.valueBuckets
                  }
                />
              )
            )}
          </g>
        ));
      }
      default: {
        return null;
      }
    }
  })();

  const heatmapContent = (() => {
    switch (exemplarsSingleView.type) {
      case 'loaded':
      case 'reloading':
        return (
          <>
            <Axis
              axis="y"
              min={exemplarsSingleView.heatmap.minValue}
              max={exemplarsSingleView.heatmap.maxValue}
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
            <svg
              role="img"
              className={styles.heatmapSvg}
              height={HEATMAP_HEIGHT}
            >
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
              min={exemplarsSingleView.heatmap.startTime}
              max={exemplarsSingleView.heatmap.endTime}
              ticksNumber={7}
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
                {exemplarsSingleView.heatmap.minDepth}
              </span>
              <span
                role="textbox"
                style={{ color: Color.rgb(COLOR_2).toString() }}
              >
                {exemplarsSingleView.heatmap.maxDepth}
              </span>
            </div>
          </>
        );
      default: {
        return null;
      }
    }
  })();

  return (
    <div
      ref={heatmapRef}
      className={styles.heatmapContainer}
      data-testid="heatmap-container"
    >
      {exemplarsSingleView.type === 'loading' ? (
        <LoadingSpinner />
      ) : (
        heatmapContent
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
