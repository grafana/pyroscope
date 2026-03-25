import { Select } from '@components/core/Select';
import { Icon } from '@components/core/Icon';
import { CascadeSelect } from '@components/core/CascadeSelect';
import { type ProfileType, type Service } from '@hooks/usePyroscopeQuery';
import { profileTypeLabel } from '@api/client';
import './NavBar.css';

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
  servicesLoading,
  service,
  profileType,
  timeRange,
  theme,
  onAppSelect,
  onTimeChange,
  onThemeChange,
}: {
  services: Service[];
  servicesLoading: boolean;
  service: string;
  profileType: ProfileType;
  timeRange: string;
  theme: 'dark' | 'light';
  onAppSelect: (s: string, pt: ProfileType) => void;
  onTimeChange: (v: string) => void;
  onThemeChange: (t: 'dark' | 'light') => void;
}) {
  return (
    <nav className="navbar">
      <div className="navbar-brand">
        <Icon name="logo" size={18} />
        <span className="navbar-brand-name">Pyroscope</span>
      </div>

      <div className="navbar-divider" />

      <CascadeSelect
        groups={services.map((s) => ({
          label: s.name,
          value: s.name,
          items: s.profileTypes.map((pt) => ({ label: profileTypeLabel(pt), value: pt })),
        }))}
        groupLabel="Service"
        itemLabel="Profile Type"
        value={{ group: service, item: profileType }}
        onChange={(g, i) => onAppSelect(g, i as ProfileType)}
        loading={servicesLoading}
      />
      <Select value={timeRange} options={TIME_PRESETS} onChange={onTimeChange} />

      <div className="navbar-spacer" />

      <Select
        value={theme}
        options={THEME_OPTIONS}
        onChange={(v) => onThemeChange(v as 'dark' | 'light')}
      />
    </nav>
  );
}
