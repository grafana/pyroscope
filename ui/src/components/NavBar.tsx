import { Select } from '@components/core/Select';
import { Icon } from '@components/core/Icon';
import { CascadeSelect } from '@components/core/CascadeSelect';
import { type ProfileType, type Service } from '@hooks/usePyroscopeQuery';
import { profileTypeLabel, sortProfileTypes } from '@api/client';
import './NavBar.css';

const ABS_VALUE = '__abs__';

function formatAbsoluteRange(start: number, end: number): string {
  const s = new Date(start);
  const e = new Date(end);
  const time = (d: Date) =>
    `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  if (s.toDateString() === e.toDateString()) return `${time(s)} – ${time(e)}`;
  const date = (d: Date) =>
    d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  return `${date(s)} ${time(s)} – ${date(e)} ${time(e)}`;
}

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
  { label: 'Dark', value: 'dark', icon: 'moon' as const },
  { label: 'Light', value: 'light', icon: 'sun' as const },
];

export function NavBar({
  services,
  servicesLoading,
  service,
  profileType,
  timeRange,
  absoluteRange,
  theme,
  queryDirty,
  onAppSelect,
  onTimeChange,
  onThemeChange,
  onReset,
  isMultiTenant,
  tenantID,
  onChangeTenant,
}: {
  services: Service[];
  servicesLoading: boolean;
  service: string;
  profileType: ProfileType;
  timeRange: string;
  absoluteRange: { start: number; end: number } | undefined;
  theme: 'dark' | 'light';
  queryDirty: boolean;
  onAppSelect: (s: string, pt: ProfileType) => void;
  onTimeChange: (v: string) => void;
  onThemeChange: (t: 'dark' | 'light') => void;
  onReset: () => void;
  isMultiTenant?: boolean;
  tenantID?: string;
  onChangeTenant?: () => void;
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
          items: sortProfileTypes(s.profileTypes).map((pt) =>
            typeof pt === 'string'
              ? { label: profileTypeLabel(pt), value: pt }
              : pt,
          ),
        }))}
        groupLabel="Service"
        itemLabel="Profile Type"
        value={{ group: service, item: profileType }}
        onChange={(g, i) => onAppSelect(g, i as ProfileType)}
        loading={servicesLoading}
      />
      <Select
        value={absoluteRange ? ABS_VALUE : timeRange}
        options={
          absoluteRange
            ? [
                {
                  label: formatAbsoluteRange(
                    absoluteRange.start,
                    absoluteRange.end,
                  ),
                  value: ABS_VALUE,
                },
                { ...TIME_PRESETS[0], divider: true },
                ...TIME_PRESETS.slice(1),
              ]
            : TIME_PRESETS
        }
        onChange={(v) => {
          if (v !== ABS_VALUE) onTimeChange(v);
        }}
      />
      {queryDirty && (
        <button className="navbar-reset" onClick={onReset}>
          Reset query
        </button>
      )}

      <div className="navbar-spacer" />

      {isMultiTenant && (
        <button className="navbar-tenant" onClick={onChangeTenant}>
          {tenantID}
        </button>
      )}

      <Select
        value={theme}
        options={THEME_OPTIONS}
        onChange={(v) => onThemeChange(v as 'dark' | 'light')}
      />
    </nav>
  );
}
