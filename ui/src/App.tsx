import { useState, useEffect } from 'react';
import './theme.css';
import { NavBar } from '@components/NavBar';
import { FlameGraph } from '@components/FlameGraph';
import { QueryBar } from '@components/QueryBar';
import { TimeSeries } from '@components/TimeSeries';
import { Panel } from '@components/Panel';
import {
  usePyroscopeQuery,
  type ProfileType,
} from '@hooks/usePyroscopeQuery';
import { profileTypeLabel } from '@api/client';

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

export default function App() {
  const { theme, setTheme } = useTheme();
  const [service, setService] = useState('');
  const [profileType, setProfileType] = useState<ProfileType>('');
  const [timeRange, setTimeRange] = useState('now-1h');
  const [queryInput, setQueryInput] = useState('');

  const query = usePyroscopeQuery({ service, profileType, timeRange });

  useEffect(() => {
    if (service || profileType) setQueryInput(buildQuery(service, profileType));
  }, [service, profileType]);

  useEffect(() => {
    if (query.servicesLoading || service) return;
    const first = query.services[0];
    if (!first) return;
    setService(first.name);
    setProfileType(first.profileTypes[0] ?? '');
  }, [query.services, query.servicesLoading, service]);

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
        servicesLoading={query.servicesLoading}
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

      {query.error && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--space-2)',
            padding: 'var(--space-2) var(--space-4)',
            background: 'var(--color-error-subtle)',
            borderBottom: '1px solid var(--color-error-border)',
            fontSize: 'var(--text-sm)',
            color: 'var(--color-error-text)',
          }}
        >
          <span>Unable to reach Pyroscope backend.</span>
          <span style={{ color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)' }}>
            {query.error}
          </span>
        </div>
      )}

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
          <TimeSeries data={query.timeline} timeRange={timeRange} />
        </Panel>

        <Panel
          title="Flamegraph"
          meta={`${service} · ${profileTypeLabel(profileType)} · ${timeRange}`}
        >
          <FlameGraph data={query.flamegraph} theme={theme} />
        </Panel>
      </div>
    </div>
  );
}
