import { useState, useRef, useEffect } from 'react';
import './theme.css';
import { Icon } from './components/core/Icon';
import { DropdownItem, DropdownSection } from './components/core/Dropdown';
import { FlameGraph } from './components/FlameGraph';
import { QueryBar } from './components/QueryBar';
import { useClickOutside } from './hooks/useClickOutside';
import {
  usePyroscopeQuery,
  type ProfileType,
  type Service,
} from './hooks/usePyroscopeQuery';

const PROFILE_LABEL: Record<ProfileType, string> = {
  cpu: 'CPU',
  memory: 'Memory',
  goroutine: 'Goroutine',
  mutex: 'Mutex',
  block: 'Block',
};

const TIME_PRESETS = [
  { label: 'Last 5m', value: 'now-5m' },
  { label: 'Last 15m', value: 'now-15m' },
  { label: 'Last 30m', value: 'now-30m' },
  { label: 'Last 1h', value: 'now-1h' },
  { label: 'Last 3h', value: 'now-3h' },
  { label: 'Last 6h', value: 'now-6h' },
  { label: 'Last 12h', value: 'now-12h' },
  { label: 'Last 24h', value: 'now-24h' },
  { label: 'Last 2d', value: 'now-2d' },
  { label: 'Last 7d', value: 'now-7d' },
  { label: 'Last 30d', value: 'now-30d' },
];

function buildQuery(service: string, pt: ProfileType): string {
  return `{service_name="${service}", profile_type="${pt}"}`;
}


function useTheme() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');
  const toggle = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    if (next === 'light')
      document.documentElement.setAttribute('data-theme', 'light');
    else document.documentElement.removeAttribute('data-theme');
  };
  return { theme, toggle };
}

function NavButton({
  children,
  onClick,
  active = false,
  title,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  active?: boolean;
  title?: string;
}) {
  const [hov, setHov] = useState(false);
  return (
    <button
      title={title}
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 'var(--space-2)',
        padding: 'var(--space-1-5) var(--space-3)',
        background: active
          ? 'var(--action-selected)'
          : hov
            ? 'var(--action-hover)'
            : 'transparent',
        color: active ? 'var(--color-primary-text)' : 'var(--text-primary)',
        border: `1px solid ${active ? 'var(--color-primary-border)' : 'transparent'}`,
        borderRadius: 'var(--radius-md)',
        fontSize: 'var(--text-md)',
        fontWeight: 'var(--weight-medium)',
        cursor: 'pointer',
        whiteSpace: 'nowrap',
        transition: 'background var(--duration-fast) var(--ease-out)',
      }}
    >
      {children}
    </button>
  );
}


function AppSelector({
  services,
  service,
  profileType,
  onSelect,
}: {
  services: Service[];
  service: string;
  profileType: ProfileType;
  onSelect: (service: string, profileType: ProfileType) => void;
}) {
  const [open, setOpen] = useState(false);
  const [hovSvc, setHovSvc] = useState(service);
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => setOpen(false));

  // Keep hovered service in sync when closed
  const handleOpen = () => {
    setHovSvc(service);
    setOpen((o) => !o);
  };

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <NavButton onClick={handleOpen} active={open}>
        {service} · {PROFILE_LABEL[profileType]}
        <Icon name="chevron-down" size={11} />
      </NavButton>

      {open && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            zIndex: 1000,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-medium)',
            borderRadius: 'var(--radius-lg)',
            boxShadow: 'var(--shadow-md)',
            display: 'flex',
            minWidth: 340,
            overflow: 'hidden',
          }}
        >
          {/* Services */}
          <div
            style={{
              width: 160,
              borderRight: '1px solid var(--border-weak)',
              flexShrink: 0,
            }}
          >
            <DropdownSection label="Service" />
            {services.map((s) => {
              const active = s.name === hovSvc;
              return (
                <div
                  key={s.name}
                  onMouseEnter={() => setHovSvc(s.name)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: 'var(--space-1-5) var(--space-3)',
                    fontSize: 'var(--text-md)',
                    color: active
                      ? 'var(--color-primary-text)'
                      : 'var(--text-primary)',
                    background: active
                      ? 'var(--action-selected)'
                      : 'transparent',
                    cursor: 'pointer',
                    borderLeft: `2px solid ${active ? 'var(--color-primary)' : 'transparent'}`,
                  }}
                >
                  {s.name}
                  {active && <Icon name="chevron-right" size={10} />}
                </div>
              );
            })}
          </div>

          {/* Profile types */}
          <div style={{ flex: 1 }}>
            <DropdownSection label="Profile Type" />
            {(services.find((s) => s.name === hovSvc)?.profileTypes ?? []).map(
              (pt) => {
                const sel = hovSvc === service && pt === profileType;
                return (
                  <DropdownItem
                    key={pt}
                    selected={sel}
                    onClick={() => {
                      onSelect(hovSvc, pt);
                      setOpen(false);
                    }}
                  >
                    <span>{PROFILE_LABEL[pt]}</span>
                    {sel && (
                      <span
                        style={{
                          fontSize: 'var(--text-xs)',
                          color: 'var(--color-primary)',
                        }}
                      >
                        ✓
                      </span>
                    )}
                  </DropdownItem>
                );
              },
            )}
          </div>
        </div>
      )}
    </div>
  );
}


function TimeRangePicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => setOpen(false));
  const label = TIME_PRESETS.find((p) => p.value === value)?.label ?? value;

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <NavButton onClick={() => setOpen((o) => !o)} active={open}>
        {label}
        <Icon name="chevron-down" size={11} />
      </NavButton>

      {open && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            zIndex: 1000,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-medium)',
            borderRadius: 'var(--radius-lg)',
            boxShadow: 'var(--shadow-md)',
            minWidth: 160,
            overflow: 'hidden',
            padding: 'var(--space-1) 0',
          }}
        >
          {TIME_PRESETS.map((p, i) => {
            const showDivider = i === 3 || i === 8;
            return (
              <div key={p.value}>
                {showDivider && (
                  <div
                    style={{
                      height: 1,
                      background: 'var(--border-weak)',
                      margin: 'var(--space-1) 0',
                    }}
                  />
                )}
                <DropdownItem
                  selected={p.value === value}
                  onClick={() => {
                    onChange(p.value);
                    setOpen(false);
                  }}
                >
                  <span>{p.label}</span>
                  {p.value === value && (
                    <span
                      style={{
                        fontSize: 'var(--text-xs)',
                        color: 'var(--color-primary)',
                        marginLeft: 'var(--space-4)',
                      }}
                    >
                      ✓
                    </span>
                  )}
                </DropdownItem>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}


function Navbar({
  services,
  service,
  profileType,
  timeRange,
  theme,
  onAppSelect,
  onTimeChange,
  onThemeToggle,
}: {
  services: Service[];
  service: string;
  profileType: ProfileType;
  timeRange: string;
  theme: 'dark' | 'light';
  onAppSelect: (s: string, pt: ProfileType) => void;
  onTimeChange: (v: string) => void;
  onThemeToggle: () => void;
}) {
  return (
    <nav
      style={{
        display: 'flex',
        alignItems: 'center',
        height: 46,
        padding: '0 var(--space-3)',
        background: 'var(--bg-canvas)',
        borderBottom: '1px solid var(--border-weak)',
        gap: 'var(--space-1)',
        flexShrink: 0,
      }}
    >
      {/* Logo */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--space-2)',
          color: 'var(--color-primary)',
          padding: '0 var(--space-2)',
          marginRight: 'var(--space-1)',
        }}
      >
        <Icon name="logo" size={18} />
        <span
          style={{
            fontSize: 'var(--text-lg)',
            fontWeight: 'var(--weight-medium)',
            color: 'var(--text-primary)',
            letterSpacing: 'var(--tracking-tight)',
          }}
        >
          Pyroscope
        </span>
      </div>

      <div
        style={{
          width: 1,
          height: 20,
          background: 'var(--border-medium)',
          margin: '0 var(--space-1)',
        }}
      />

      <AppSelector
        services={services}
        service={service}
        profileType={profileType}
        onSelect={onAppSelect}
      />
      <TimeRangePicker value={timeRange} onChange={onTimeChange} />

      <div style={{ flex: 1 }} />

      <NavButton
        onClick={onThemeToggle}
        title={theme === 'dark' ? 'Light mode' : 'Dark mode'}
      >
        {theme === 'dark' ? (
          <Icon name="sun" size={14} />
        ) : (
          <Icon name="moon" size={14} />
        )}
      </NavButton>
    </nav>
  );
}


function TimelineChart({ data }: { data: number[] }) {
  const W = 800,
    H = 72;
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

  return (
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
      <path d={area} fill="url(#tl-fill)" />
      <path
        d={line}
        fill="none"
        stroke="#3d71d9"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
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
  const { theme, toggle } = useTheme();
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
      <Navbar
        services={query.services}
        service={service}
        profileType={profileType}
        timeRange={timeRange}
        theme={theme}
        onAppSelect={handleAppSelect}
        onTimeChange={setTimeRange}
        onThemeToggle={toggle}
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
          <TimelineChart data={query.timeline} />
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
