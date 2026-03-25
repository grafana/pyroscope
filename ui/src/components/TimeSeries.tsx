import './TimeSeries.css';

const TICK_INTERVALS_MS = [
  60_000,
  5 * 60_000,
  10 * 60_000,
  15 * 60_000,
  30 * 60_000,
  3_600_000,
  2 * 3_600_000,
  4 * 3_600_000,
  6 * 3_600_000,
  12 * 3_600_000,
  86_400_000,
  2 * 86_400_000,
  7 * 86_400_000,
];

function parseRangeMs(range: string): number {
  const m = range.match(/^now-(\d+)([mhd])$/);
  if (!m) return 3_600_000;
  const mult: Record<string, number> = { m: 60_000, h: 3_600_000, d: 86_400_000 };
  return parseInt(m[1]) * (mult[m[2]] ?? 60_000);
}

function niceTickInterval(durationMs: number): number {
  const target = durationMs / 5;
  return TICK_INTERVALS_MS.find((i) => i >= target) ?? TICK_INTERVALS_MS[TICK_INTERVALS_MS.length - 1];
}

function formatTickLabel(msFromNow: number): string {
  if (msFromNow === 0) return 'now';
  const abs = Math.abs(msFromNow);
  if (abs >= 86_400_000 && abs % 86_400_000 === 0) return `-${abs / 86_400_000}d`;
  if (abs >= 3_600_000 && abs % 3_600_000 === 0) return `-${abs / 3_600_000}h`;
  return `-${abs / 60_000}m`;
}

export function TimeSeries({ data, timeRange }: { data: number[]; timeRange: string }) {
  const W = 800,
    H = 100;
  const n = data.length;

  if (n === 0) {
    return (
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} className="timeseries-svg" />
    );
  }

  const pts = data.map(
    (v, i) => [(i / Math.max(n - 1, 1)) * W, H - 4 - v * (H - 10)] as [number, number],
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

  const durationMs = parseRangeMs(timeRange);
  const tickInterval = niceTickInterval(durationMs);
  const ticks: { x: number; label: string }[] = [];
  for (let t = 0; t <= durationMs; t += tickInterval) {
    ticks.push({ x: (t / durationMs) * W, label: formatTickLabel(t - durationMs) });
  }
  if (ticks[ticks.length - 1].x < W) {
    ticks.push({ x: W, label: 'now' });
  }

  const yTicks = [0, 0.5, 1].map((v) => ({
    y: H - 4 - v * (H - 10),
    label: v === 0 ? '0%' : v === 1 ? '100%' : '50%',
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
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          width="100%"
          height={H}
          className="timeseries-svg"
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
              x1={0} y1={y.toFixed(1)}
              x2={W} y2={y.toFixed(1)}
              stroke="var(--border-medium)"
              strokeWidth="1"
              vectorEffect="non-scaling-stroke"
            />
          ))}
          {ticks.map(({ x }) => (
            <line
              key={x}
              x1={x.toFixed(1)} y1={0}
              x2={x.toFixed(1)} y2={H}
              stroke="var(--border-medium)"
              strokeWidth="1"
              vectorEffect="non-scaling-stroke"
            />
          ))}
          <line
            x1={0} y1={H} x2={W} y2={H}
            stroke="var(--border-medium)"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
          <line
            x1={0} y1={0} x2={0} y2={H}
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
        </svg>
        <div className="timeseries-x-axis">
          {ticks.map(({ x, label }) => (
            <span
              key={x}
              className="timeseries-x-label"
              style={{
                left: `${(x / W) * 100}%`,
                transform:
                  x === 0 ? 'none'
                  : x === W ? 'translateX(-100%)'
                  : 'translateX(-50%)',
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
