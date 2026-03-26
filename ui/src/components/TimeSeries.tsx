import { useEffect, useRef, useState } from 'react';
import { Empty } from '@components/core/Empty';
import { profileTypeUnit } from '@api/client';
import './TimeSeries.css';

function toDisplayValue(raw: number, unit: string): number {
  if (unit === 'ns') return raw / 1e9;
  return raw;
}

function niceMax(value: number): number {
  if (value <= 0) return 1;
  const exp = Math.floor(Math.log10(value));
  const mag = Math.pow(10, exp);
  const norm = value / mag;
  if (norm <= 1) return mag;
  if (norm <= 2) return 2 * mag;
  if (norm <= 5) return 5 * mag;
  return 10 * mag;
}

function yAxisFormatter(displayMax: number): (v: number) => string {
  let divisor = 1,
    suffix = '';
  if (displayMax >= 1e9) {
    divisor = 1e9;
    suffix = 'G';
  } else if (displayMax >= 1e6) {
    divisor = 1e6;
    suffix = 'M';
  } else if (displayMax >= 1e3) {
    divisor = 1e3;
    suffix = 'k';
  } else if (displayMax < 1e-3 && displayMax > 0) {
    divisor = 1e-6;
    suffix = 'µ';
  } else if (displayMax < 1 && displayMax > 0) {
    divisor = 1e-3;
    suffix = 'm';
  }
  return (v: number) => {
    if (v === 0) return '0';
    return `${parseFloat((v / divisor).toPrecision(3))}${suffix}`;
  };
}

function parseRangeMs(range: string): number {
  const m = range.match(/^now-(\d+)([mhd])$/);
  if (!m) return 3_600_000;
  const mult: Record<string, number> = {
    m: 60_000,
    h: 3_600_000,
    d: 86_400_000,
  };
  return parseInt(m[1]) * (mult[m[2]] ?? 60_000);
}

function tickStepMs(durationMs: number): number {
  const m = 60_000,
    h = 3_600_000,
    d = 86_400_000;
  if (durationMs <= 15 * m) return m;
  if (durationMs <= 2 * h) return 5 * m;
  if (durationMs <= 4 * h) return 15 * m;
  if (durationMs <= 8 * h) return 30 * m;
  if (durationMs <= 12 * h) return h;
  if (durationMs <= d) return 2 * h;
  if (durationMs <= 7 * d) return 12 * h;
  return d;
}

function formatTickTime(ts: number, stepMs: number): string {
  const d = new Date(ts);
  if (stepMs >= 86_400_000) {
    return d.toLocaleDateString(undefined, {
      month: 'numeric',
      day: 'numeric',
    });
  }
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

export function TimeSeries({
  data,
  timeRange,
  profileTypeId,
  startMs,
  endMs,
  onRangeSelect,
}: {
  data: number[];
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

  const max = Math.max(...data);
  const norm = max === 0 ? data.map(() => 0) : data.map((v) => v / max);

  const pts = norm.map(
    (v, i) =>
      [(i / Math.max(n - 1, 1)) * W, H - 4 - v * (H - 10)] as [number, number],
  );

  const area =
    `M 0,${H} ` +
    pts.map(([x, y]) => `L ${x.toFixed(1)},${y.toFixed(1)}`).join(' ') +
    ` L ${W},${H} Z`;

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
