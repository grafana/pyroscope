import { Select } from '@components/core/Select';
import { Icon } from '@components/core/Icon';
import { CascadeSelect } from '@components/core/CascadeSelect';
import { type ProfileType, type Service } from '@hooks/usePyroscopeQuery';

const PROFILE_LABEL: Record<ProfileType, string> = {
  cpu: 'cpu',
  memory: 'memory',
  goroutine: 'goroutine',
  mutex: 'mutex',
  block: 'block',
};

const TIME_PRESETS = [
  { label: 'Last 5m', value: 'now-5m' },
  { label: 'Last 15m', value: 'now-15m' },
  { label: 'Last 30m', value: 'now-30m' },
  { label: 'Last 1h', value: 'now-1h', divider: true },
  { label: 'Last 3h', value: 'now-3h' },
  { label: 'Last 6h', value: 'now-6h' },
  { label: 'Last 12h', value: 'now-12h' },
  { label: 'Last 24h', value: 'now-24h' },
  { label: 'Last 2d', value: 'now-2d', divider: true },
  { label: 'Last 7d', value: 'now-7d' },
  { label: 'Last 30d', value: 'now-30d' },
];

const THEME_OPTIONS = [
  { label: 'Dark', value: 'dark' },
  { label: 'Light', value: 'light' },
];

export function NavBar({
  services,
  service,
  profileType,
  timeRange,
  theme,
  onAppSelect,
  onTimeChange,
  onThemeChange,
}: {
  services: Service[];
  service: string;
  profileType: ProfileType;
  timeRange: string;
  theme: 'dark' | 'light';
  onAppSelect: (s: string, pt: ProfileType) => void;
  onTimeChange: (v: string) => void;
  onThemeChange: (t: 'dark' | 'light') => void;
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

      <CascadeSelect
        groups={services.map((s) => ({
          label: s.name,
          value: s.name,
          items: s.profileTypes.map((pt) => ({ label: PROFILE_LABEL[pt], value: pt })),
        }))}
        groupLabel="Service"
        itemLabel="Profile Type"
        value={{ group: service, item: profileType }}
        onChange={(g, i) => onAppSelect(g, i as ProfileType)}
      />
      <Select value={timeRange} options={TIME_PRESETS} onChange={onTimeChange} />

      <div style={{ flex: 1 }} />

      <Select
        value={theme}
        options={THEME_OPTIONS}
        onChange={(v) => onThemeChange(v as 'dark' | 'light')}
      />
    </nav>
  );
}
