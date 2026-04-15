import { useEffect, useRef, useState } from 'react';
import { Empty } from '@components/core/Empty';
import { profileTypeUnit } from '@api/client';
import {
  toDisplayValue,
  niceMax,
  yAxisFormatter,
  parseRangeMs,
  tickStepMs,
  formatTickTime,
} from './timeseries-utils';
import './TimeSeries.css';

export function TimeSeries({
  data,
  timeRange,
  profileTypeId,
  startMs,
  endMs,
  onRangeSelect,
}: {
  data: { value: number; timestamp: number }[];
  timeRange: string;
  profileTypeId: string;
  startMs?: number;
  endMs?: number;
  onRangeSelect: (start: number, end: number) => void;
}) {
  const W = 800,
    H = 100;
  const n = data.length;

  const svgRef = useRef<SVGSVGElement>(null);
  const [drag, setDrag] = useState<{
    startFrac: number;
    currentFrac: number;
  } | null>(null);
  const dragRef = useRef(drag);
  dragRef.current = drag;
  const timeRef = useRef({ rangeStart: 0, durationMs: 0, onRangeSelect });

  const isDragging = drag !== null;

  useEffect(() => {
    if (!isDragging) return;
    const getX = (e: MouseEvent) => {
      const rect = svgRef.current?.getBoundingClientRect();
      if (!rect) return 0;
      return Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    };
    const onMove = (e: MouseEvent) => {
      setDrag((prev) => (prev ? { ...prev, currentFrac: getX(e) } : null));
    };
    const onUp = (e: MouseEvent) => {
      const frac = getX(e);
      const startFrac = dragRef.current!.startFrac;
      const { rangeStart, durationMs, onRangeSelect } = timeRef.current;
      const lo = Math.min(startFrac, frac);
      const hi = Math.max(startFrac, frac);
      if (hi - lo > 0.005) {
        onRangeSelect(
          Math.round(rangeStart + lo * durationMs),
          Math.round(rangeStart + hi * durationMs),
        );
      }
      setDrag(null);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  }, [isDragging]);

  if (n === 0) {
    return <Empty />;
  }

  // eslint-disable-next-line react-hooks/purity
  const rangeEnd = endMs ?? Date.now();
  const durationMs =
    startMs != null && endMs != null
      ? endMs - startMs
      : parseRangeMs(timeRange);
  const rangeStart = rangeEnd - durationMs;
  timeRef.current = { rangeStart, durationMs, onRangeSelect };

  const max = Math.max(...data.map((d) => d.value));
  const norm = max === 0 ? data.map(() => 0) : data.map((d) => d.value / max);

  const pts = norm.map(
    (v, i) =>
      [
        ((data[i].timestamp - rangeStart) / durationMs) * W,
        H - 4 - v * (H - 10),
      ] as [number, number],
  );

  const area =
    `M ${pts[0][0].toFixed(1)},${H} ` +
    pts.map(([x, y]) => `L ${x.toFixed(1)},${y.toFixed(1)}`).join(' ') +
    ` L ${pts[pts.length - 1][0].toFixed(1)},${H} Z`;

  const line =
    `M ${pts[0][0].toFixed(1)},${pts[0][1].toFixed(1)} ` +
    pts
      .slice(1)
      .map(([x, y]) => `L ${x.toFixed(1)},${y.toFixed(1)}`)
      .join(' ');

  const stepMs = tickStepMs(durationMs);

  const tickMap = new Map<
    number,
    { x: number; label: string; midnight: boolean }
  >();

  const firstTick = Math.ceil(rangeStart / stepMs) * stepMs;
  for (let ts = firstTick; ts <= rangeEnd; ts += stepMs) {
    tickMap.set(ts, {
      x: ((ts - rangeStart) / durationMs) * W,
      label: formatTickTime(ts, stepMs),
      midnight: false,
    });
  }

  const firstMidnight = new Date(rangeStart);
  firstMidnight.setHours(0, 0, 0, 0);
  if (firstMidnight.getTime() <= rangeStart)
    firstMidnight.setDate(firstMidnight.getDate() + 1);
  for (
    const d = new Date(firstMidnight);
    d.getTime() <= rangeEnd;
    d.setDate(d.getDate() + 1)
  ) {
    const ts = d.getTime();
    const x = ((ts - rangeStart) / durationMs) * W;
    const label = d.toLocaleDateString(undefined, {
      month: 'numeric',
      day: 'numeric',
    });
    tickMap.set(ts, { x, label, midnight: true });
  }

  const ticks = Array.from(tickMap.values()).sort((a, b) => a.x - b.x);

  const unit = profileTypeUnit(profileTypeId);
  const displayMax = niceMax(toDisplayValue(max, unit));
  const fmt = yAxisFormatter(displayMax);
  const yTicks = [0, 0.5, 1].map((v) => ({
    y: H - 4 - v * (H - 10),
    label: fmt(v * displayMax),
  }));

  return (
    <div className="timeseries">
      <div className="timeseries-y-axis">
        {yTicks.map(({ y, label }) => (
          <span key={label} className="timeseries-y-label" style={{ top: y }}>
            {label}
          </span>
        ))}
      </div>
      <div className="timeseries-chart">
        <svg
          ref={svgRef}
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          width="100%"
          height={H}
          className="timeseries-svg"
          style={{ cursor: durationMs < 5 * 60_000 ? 'default' : 'crosshair' }}
          onMouseDown={(e) => {
            if (e.button !== 0 || durationMs < 5 * 60_000) return;
            e.preventDefault();
            const rect = e.currentTarget.getBoundingClientRect();
            const frac = Math.max(
              0,
              Math.min(1, (e.clientX - rect.left) / rect.width),
            );
            setDrag({ startFrac: frac, currentFrac: frac });
          }}
        >
          <defs>
            <linearGradient id="tl-fill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#3d71d9" stopOpacity="0.30" />
              <stop offset="100%" stopColor="#3d71d9" stopOpacity="0.02" />
            </linearGradient>
          </defs>
          {yTicks.map(({ y }) => (
            <line
              key={y}
              x1={0}
              y1={y.toFixed(1)}
              x2={W}
              y2={y.toFixed(1)}
              stroke="var(--border-medium)"
              strokeWidth="1"
              vectorEffect="non-scaling-stroke"
            />
          ))}
          {ticks.map(({ x, midnight }) => (
            <line
              key={x}
              x1={x.toFixed(1)}
              y1={0}
              x2={x.toFixed(1)}
              y2={H}
              stroke={
                midnight ? 'var(--border-strong)' : 'var(--border-medium)'
              }
              strokeWidth="1"
              vectorEffect="non-scaling-stroke"
            />
          ))}
          <line
            x1={0}
            y1={H}
            x2={W}
            y2={H}
            stroke="var(--border-medium)"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
          <line
            x1={0}
            y1={0}
            x2={0}
            y2={H}
            stroke="var(--border-medium)"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
          <path d={area} fill="url(#tl-fill)" />
          <path
            d={line}
            fill="none"
            stroke="#3d71d9"
            strokeWidth="1.5"
            vectorEffect="non-scaling-stroke"
          />
          {drag &&
            (() => {
              const x1 = Math.min(drag.startFrac, drag.currentFrac) * W;
              const x2 = Math.max(drag.startFrac, drag.currentFrac) * W;
              return (
                <rect
                  x={x1}
                  y={0}
                  width={x2 - x1}
                  height={H}
                  fill="var(--color-primary)"
                  opacity={0.2}
                  style={{ pointerEvents: 'none' }}
                />
              );
            })()}
        </svg>
        <div className="timeseries-x-axis">
          {ticks.map(({ x, label, midnight }) => (
            <span
              key={x}
              className="timeseries-x-label"
              style={{
                left: `${(x / W) * 100}%`,
                transform:
                  x <= 0
                    ? 'none'
                    : x >= W
                      ? 'translateX(-100%)'
                      : 'translateX(-50%)',
                color: midnight ? 'var(--text-primary)' : undefined,
              }}
            >
              {label}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
