import { useState, useRef, useEffect } from 'react';
import './theme.css';
import { Icon } from './components/Icon';
import {
  usePyroscopeQuery,
  type ProfileType,
  type Frame,
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

function djb2(s: string) {
  let h = 5381;
  for (let i = 0; i < s.length; i++)
    h = ((h * 33) ^ s.charCodeAt(i)) & 0x7fffffff;
  return h;
}

function frameColor(name: string): string {
  const h = djb2(name);
  if (/gc|GC|grey|malloc/.test(name))
    return `hsl(${2 + (h % 8)},  65%, ${32 + (h % 10)}%)`;
  if (name.startsWith('runtime.'))
    return `hsl(${20 + (h % 12)}, 68%, ${34 + (h % 10)}%)`;
  return `hsl(${28 + (h % 22)}, 72%, ${36 + (h % 10)}%)`;
}

function buildQuery(
  service: string,
  pt: ProfileType,
  filters: Record<string, string>,
): string {
  const parts = [
    `service_name="${service}"`,
    `profile_type="${pt}"`,
    ...Object.entries(filters).map(([k, v]) => `${k}="${v}"`),
  ];
  return `{${parts.join(', ')}}`;
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

function useClickOutside(
  ref: React.RefObject<HTMLElement | null>,
  cb: () => void,
) {
  const cbRef = useRef(cb);
  useEffect(() => {
    cbRef.current = cb;
  });
  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node))
        cbRef.current();
    };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, [ref]);
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


function DropdownItem({
  children,
  onClick,
  selected,
  mono,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  selected?: boolean;
  mono?: boolean;
}) {
  const [hov, setHov] = useState(false);
  return (
    <div
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: 'var(--space-1-5) var(--space-3)',
        fontSize: 'var(--text-md)',
        fontFamily: mono ? 'var(--font-mono)' : undefined,
        color: selected ? 'var(--color-primary-text)' : 'var(--text-primary)',
        background: selected
          ? 'var(--action-selected)'
          : hov
            ? 'var(--action-hover)'
            : 'transparent',
        cursor: 'pointer',
      }}
    >
      {children}
    </div>
  );
}


function DropdownSection({ label }: { label: string }) {
  return (
    <div
      style={{
        padding: 'var(--space-1-5) var(--space-3) var(--space-1)',
        fontSize: 'var(--text-xs)',
        color: 'var(--text-secondary)',
        letterSpacing: 'var(--tracking-caps)',
        textTransform: 'uppercase' as const,
        borderBottom: '1px solid var(--border-weak)',
      }}
    >
      {label}
    </div>
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
        <span style={{ color: 'var(--text-primary)' }}>{service}</span>
        <span style={{ color: 'var(--text-disabled)' }}>·</span>
        <span style={{ color: 'var(--color-primary-text)' }}>
          {PROFILE_LABEL[profileType]}
        </span>
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
  onRefresh,
  onThemeToggle,
}: {
  services: Service[];
  service: string;
  profileType: ProfileType;
  timeRange: string;
  theme: 'dark' | 'light';
  onAppSelect: (s: string, pt: ProfileType) => void;
  onTimeChange: (v: string) => void;
  onRefresh: () => void;
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
      <NavButton onClick={onRefresh} title="Refresh">
        <Icon name="refresh" size={14} />
      </NavButton>

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


function FilterChip({
  label,
  value,
  onRemove,
}: {
  label: string;
  value: string;
  onRemove: () => void;
}) {
  const [hov, setHov] = useState(false);
  return (
    <div
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        height: 26,
        border: '1px solid var(--border-medium)',
        borderRadius: 'var(--radius-sm)',
        overflow: 'hidden',
        fontSize: 'var(--text-sm)',
        fontFamily: 'var(--font-mono)',
        flexShrink: 0,
      }}
    >
      <span
        style={{
          padding: '0 var(--space-2)',
          color: 'var(--text-secondary)',
          background: 'var(--bg-secondary)',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
        }}
      >
        {label}
      </span>
      <span
        style={{
          padding: '0 var(--space-1)',
          color: 'var(--text-disabled)',
          background: 'var(--bg-secondary)',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
        }}
      >
        =
      </span>
      <span
        style={{
          padding: '0 var(--space-2)',
          color: 'var(--color-primary-text)',
          background: 'var(--color-primary-subtle)',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
        }}
      >
        {value}
      </span>
      <button
        onClick={onRemove}
        onMouseEnter={() => setHov(true)}
        onMouseLeave={() => setHov(false)}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          width: 24,
          height: '100%',
          background: hov ? 'var(--color-error-subtle)' : 'var(--bg-secondary)',
          color: hov ? 'var(--color-error-text)' : 'var(--text-disabled)',
          border: 'none',
          borderLeft: '1px solid var(--border-weak)',
          cursor: 'pointer',
          padding: 0,
          transition:
            'background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out)',
        }}
      >
        <Icon name="x" size={9} />
      </button>
    </div>
  );
}


function LabelFilter({
  labels,
  activeFilters,
  onAdd,
}: {
  labels: Record<string, string[]>;
  activeFilters: Record<string, string>;
  onAdd: (label: string, value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [step, setStep] = useState<'labels' | string>('labels');
  const ref = useRef<HTMLDivElement>(null);
  useClickOutside(ref, () => {
    setOpen(false);
    setStep('labels');
  });

  const available = Object.keys(labels).filter((k) => !(k in activeFilters));
  const disabled = available.length === 0;

  const close = () => {
    setOpen(false);
    setStep('labels');
  };
  const [btnHov, setBtnHov] = useState(false);

  return (
    <div ref={ref} style={{ position: 'relative', flexShrink: 0 }}>
      <button
        onClick={() => {
          if (!disabled) {
            setOpen((o) => !o);
            setStep('labels');
          }
        }}
        onMouseEnter={() => setBtnHov(true)}
        onMouseLeave={() => setBtnHov(false)}
        disabled={disabled}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 'var(--space-1)',
          height: 26,
          padding: '0 var(--space-2)',
          background: open
            ? 'var(--action-selected)'
            : btnHov
              ? 'var(--action-hover)'
              : 'transparent',
          color: open ? 'var(--color-primary-text)' : 'var(--text-secondary)',
          border: `1px solid ${open ? 'var(--color-primary-border)' : 'var(--border-medium)'}`,
          borderRadius: 'var(--radius-sm)',
          fontSize: 'var(--text-sm)',
          cursor: disabled ? 'not-allowed' : 'pointer',
          opacity: disabled ? 0.4 : 1,
          transition: 'background var(--duration-fast) var(--ease-out)',
        }}
      >
        <Icon name="plus" size={11} />
        Filter
      </button>

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
            minWidth: 200,
            overflow: 'hidden',
          }}
        >
          {step === 'labels' ? (
            <>
              <DropdownSection label="Label" />
              {available.map((lbl) => (
                <DropdownItem key={lbl} mono onClick={() => setStep(lbl)}>
                  <span>{lbl}</span>
                  <Icon name="chevron-right" size={10} />
                </DropdownItem>
              ))}
            </>
          ) : (
            <>
              <div
                onClick={() => setStep('labels')}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 'var(--space-2)',
                  padding: 'var(--space-1-5) var(--space-3)',
                  borderBottom: '1px solid var(--border-weak)',
                  cursor: 'pointer',
                  color: 'var(--text-secondary)',
                  fontSize: 'var(--text-xs)',
                  letterSpacing: 'var(--tracking-caps)',
                  textTransform: 'uppercase' as const,
                }}
              >
                <Icon name="chevron-left" size={10} />
                {step}
              </div>
              {(labels[step] ?? []).map((val) => (
                <DropdownItem
                  key={val}
                  mono
                  onClick={() => {
                    onAdd(step, val);
                    close();
                  }}
                >
                  {val}
                </DropdownItem>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  );
}


function TagsBar({
  labels,
  filters,
  query,
  onAddFilter,
  onRemoveFilter,
  onQueryChange,
  onRun,
}: {
  labels: Record<string, string[]>;
  filters: Record<string, string>;
  query: string;
  onAddFilter: (label: string, value: string) => void;
  onRemoveFilter: (label: string) => void;
  onQueryChange: (q: string) => void;
  onRun: () => void;
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        height: 44,
        padding: '0 var(--space-3)',
        background: 'var(--bg-primary)',
        borderBottom: '1px solid var(--border-weak)',
        gap: 'var(--space-2)',
        flexShrink: 0,
      }}
    >
      <LabelFilter
        labels={labels}
        activeFilters={filters}
        onAdd={onAddFilter}
      />

      {Object.entries(filters).map(([k, v]) => (
        <FilterChip
          key={k}
          label={k}
          value={v}
          onRemove={() => onRemoveFilter(k)}
        />
      ))}

      <input
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && onRun()}
        onFocus={(e) => {
          e.currentTarget.style.borderColor = 'var(--color-primary-border)';
          e.currentTarget.style.boxShadow = '0 0 0 2px var(--action-focus)';
        }}
        onBlur={(e) => {
          e.currentTarget.style.borderColor = 'var(--border-medium)';
          e.currentTarget.style.boxShadow = 'none';
        }}
        style={{
          flex: 1,
          height: 28,
          background: 'var(--bg-secondary)',
          color: 'var(--text-primary)',
          border: '1px solid var(--border-medium)',
          borderRadius: 'var(--radius-sm)',
          padding: '0 var(--space-3)',
          fontSize: 'var(--text-sm)',
          fontFamily: 'var(--font-mono)',
          outline: 'none',
          minWidth: 0,
          transition:
            'border-color var(--duration-base) var(--ease-out), box-shadow var(--duration-base) var(--ease-out)',
        }}
      />

      <button
        onClick={onRun}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 'var(--space-1-5)',
          height: 28,
          padding: '0 var(--space-3)',
          background: 'var(--color-primary)',
          color: 'var(--color-primary-foreground)',
          border: '1px solid transparent',
          borderRadius: 'var(--radius-sm)',
          fontSize: 'var(--text-sm)',
          fontWeight: 'var(--weight-medium)',
          cursor: 'pointer',
          flexShrink: 0,
        }}
      >
        <Icon name="play" size={10} />
        Run
      </button>
    </div>
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


function FlameGraph({ levels }: { levels: Frame[][] }) {
  const [hovered, setHovered] = useState<{ name: string; pct: number } | null>(
    null,
  );
  const FRAME_H = 22;

  return (
    <div>
      {levels.map((level, li) => (
        <div
          key={li}
          style={{ position: 'relative', height: FRAME_H, marginBottom: 1 }}
        >
          {level.map((frame) => {
            const isHov = hovered?.name === frame.name;
            return (
              <div
                key={`${li}-${frame.name}`}
                onMouseEnter={() =>
                  setHovered({ name: frame.name, pct: frame.width })
                }
                onMouseLeave={() => setHovered(null)}
                style={{
                  position: 'absolute',
                  left: `${frame.start}%`,
                  width: `calc(${frame.width}% - 1px)`,
                  height: '100%',
                  background: frameColor(frame.name),
                  filter: isHov ? 'brightness(1.25)' : undefined,
                  cursor: 'pointer',
                  overflow: 'hidden',
                  display: 'flex',
                  alignItems: 'center',
                  paddingLeft: 4,
                  borderRadius: 1,
                }}
              >
                {frame.width > 2.5 && (
                  <span
                    style={{
                      fontSize: 'var(--text-xs)',
                      color: 'rgba(255,255,255,0.88)',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      pointerEvents: 'none',
                      userSelect: 'none' as const,
                      lineHeight: 1,
                    }}
                  >
                    {frame.name}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      ))}

      {/* Status bar */}
      <div
        style={{
          marginTop: 'var(--space-2)',
          paddingTop: 'var(--space-2)',
          borderTop: '1px solid var(--border-weak)',
          minHeight: 20,
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--space-3)',
        }}
      >
        {hovered ? (
          <>
            <span
              style={{
                fontSize: 'var(--text-xs)',
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-primary)',
              }}
            >
              {hovered.name}
            </span>
            <span
              style={{
                fontSize: 'var(--text-xs)',
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-secondary)',
              }}
            >
              {hovered.pct}%
            </span>
          </>
        ) : (
          <span
            style={{
              fontSize: 'var(--text-xs)',
              color: 'var(--text-disabled)',
            }}
          >
            Hover a frame to inspect
          </span>
        )}
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
  const { theme, toggle } = useTheme();
  const [service, setService] = useState('api-server');
  const [profileType, setProfileType] = useState<ProfileType>('cpu');
  const [timeRange, setTimeRange] = useState('now-1h');
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [queryInput, setQueryInput] = useState(() =>
    buildQuery('api-server', 'cpu', {}),
  );

  const query = usePyroscopeQuery({ service, profileType, timeRange, filters });

  useEffect(() => {
    setQueryInput(buildQuery(service, profileType, filters));
  }, [service, profileType, filters]);

  const handleAppSelect = (s: string, pt: ProfileType) => {
    setService(s);
    setProfileType(pt);
    setFilters({});
  };

  const handleAddFilter = (label: string, value: string) => {
    setFilters((f) => ({ ...f, [label]: value }));
  };

  const handleRemoveFilter = (label: string) => {
    setFilters((f) => {
      const n = { ...f };
      delete n[label];
      return n;
    });
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
        onRefresh={query.run}
        onThemeToggle={toggle}
      />
      <TagsBar
        labels={query.labels}
        filters={filters}
        query={queryInput}
        onAddFilter={handleAddFilter}
        onRemoveFilter={handleRemoveFilter}
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
