import { useState, useEffect } from 'react';
import './theme.css';
import { NavBar } from '@components/NavBar';
import { FlameGraph } from '@components/FlameGraph';
import { QueryBar } from '@components/QueryBar';
import {
  usePyroscopeQuery,
  type ProfileType,
} from '@hooks/usePyroscopeQuery';

const PROFILE_LABEL: Record<ProfileType, string> = {
  cpu: 'cpu',
  memory: 'memory',
  goroutine: 'goroutine',
  mutex: 'mutex',
  block: 'block',
};

function useTheme() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');
  const setAndApply = (next: 'dark' | 'light') => {
    setTheme(next);
    if (next === 'light')
      document.documentElement.setAttribute('data-theme', 'light');
    else document.documentElement.removeAttribute('data-theme');
  };
  return { theme, setTheme: setAndApply };
}

function buildQuery(service: string, pt: ProfileType): string {
  return `{service_name="${service}", profile_type="${pt}"}`;
}




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

function TimelineChart({ data, timeRange }: { data: number[]; timeRange: string }) {
  const W = 800,
    H = 56;
  const n = data.length;

  const pts = data.map(
    (v, i) => [(i / (n - 1)) * W, H - 4 - v * (H - 10)] as [number, number],
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
    <div style={{ display: 'flex' }}>
      <div style={{ width: 32, position: 'relative', flexShrink: 0 }}>
        {yTicks.map(({ y, label }) => (
          <span
            key={label}
            style={{
              position: 'absolute',
              right: 6,
              top: y,
              transform: 'translateY(-50%)',
              fontSize: 'var(--text-xs)',
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-disabled)',
              lineHeight: 1,
            }}
          >
            {label}
          </span>
        ))}
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          width="100%"
          height={H}
          style={{ display: 'block' }}
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
        <div style={{ position: 'relative', height: 18, marginTop: 2 }}>
          {ticks.map(({ x, label }) => (
            <span
              key={x}
              style={{
                position: 'absolute',
                left: `${(x / W) * 100}%`,
                transform:
                  x === 0 ? 'none'
                  : x === W ? 'translateX(-100%)'
                  : 'translateX(-50%)',
                fontSize: 'var(--text-xs)',
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-disabled)',
                whiteSpace: 'nowrap',
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


function Panel({
  title,
  meta,
  children,
}: {
  title: string;
  meta?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div
      style={{
        background: 'var(--bg-primary)',
        border: '1px solid var(--border-medium)',
        borderRadius: 'var(--radius-lg)',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: 'var(--space-2) var(--space-4)',
          borderBottom: '1px solid var(--border-weak)',
        }}
      >
        <span
          style={{
            fontSize: 'var(--text-xs)',
            fontWeight: 'var(--weight-medium)',
            color: 'var(--text-secondary)',
            letterSpacing: 'var(--tracking-caps)',
            textTransform: 'uppercase' as const,
          }}
        >
          {title}
        </span>
        {meta && (
          <span
            style={{
              fontSize: 'var(--text-xs)',
              color: 'var(--text-disabled)',
              fontFamily: 'var(--font-mono)',
            }}
          >
            {meta}
          </span>
        )}
      </div>
      <div style={{ padding: 'var(--space-3) var(--space-4)' }}>{children}</div>
    </div>
  );
}


export default function App() {
  const { theme, setTheme } = useTheme();
  const [service, setService] = useState('api-server');
  const [profileType, setProfileType] = useState<ProfileType>('cpu');
  const [timeRange, setTimeRange] = useState('now-1h');
  const [queryInput, setQueryInput] = useState(() =>
    buildQuery('api-server', 'cpu'),
  );

  const query = usePyroscopeQuery({ service, profileType, timeRange });

  useEffect(() => {
    setQueryInput(buildQuery(service, profileType));
  }, [service, profileType]);

  const handleAppSelect = (s: string, pt: ProfileType) => {
    setService(s);
    setProfileType(pt);
  };

  return (
    <div
      style={{
        minHeight: '100dvh',
        display: 'flex',
        flexDirection: 'column',
        background: 'var(--bg-canvas)',
      }}
    >
      <NavBar
        services={query.services}
        service={service}
        profileType={profileType}
        timeRange={timeRange}
        theme={theme}
        onAppSelect={handleAppSelect}
        onTimeChange={setTimeRange}
        onThemeChange={setTheme}
      />
      <QueryBar
        query={queryInput}
        onQueryChange={setQueryInput}
        onRun={query.run}
      />

      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          gap: 'var(--space-3)',
          padding: 'var(--space-3)',
        }}
      >
        <Panel title="Rate · samples / sec">
          <TimelineChart data={query.timeline} timeRange={timeRange} />
        </Panel>

        <Panel
          title="Flamegraph"
          meta={`${service} · ${PROFILE_LABEL[profileType]} · ${timeRange}`}
        >
          <FlameGraph levels={query.flamegraph} />
        </Panel>
      </div>
    </div>
  );
}
