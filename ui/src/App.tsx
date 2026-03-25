import { useState, useEffect } from 'react';
import './theme.css';
import './App.css';
import { NavBar } from '@components/NavBar';
import { FlameGraph } from '@components/FlameGraph';
import { QueryBar } from '@components/QueryBar';
import { TimeSeries } from '@components/TimeSeries';
import { Panel } from '@components/Panel';
import {
  usePyroscopeQuery,
  type ProfileType,
} from '@hooks/usePyroscopeQuery';
import { profileTypeLabel, profileTypeRateLabel, sortProfileTypes } from '@api/client';

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

function parseQuery(q: string): { service: string; profileType: string } | null {
  const service = q.match(/service_name\s*=\s*"([^"]+)"/)?.[1];
  const profileType = q.match(/profile_type\s*=\s*"([^"]+)"/)?.[1];
  if (!service || !profileType) return null;
  return { service, profileType };
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
    setProfileType(sortProfileTypes(first.profileTypes).find((pt): pt is string => typeof pt === 'string') ?? '');
  }, [query.services, query.servicesLoading, service]);

  const handleAppSelect = (s: string, pt: ProfileType) => {
    setService(s);
    setProfileType(pt);
  };

  const queryDirty = !!service && queryInput !== buildQuery(service, profileType);
  const handleReset = () => setQueryInput(buildQuery(service, profileType));

  return (
    <div className="app">
      <NavBar
        services={query.services}
        servicesLoading={query.servicesLoading}
        service={service}
        profileType={profileType}
        timeRange={timeRange}
        theme={theme}
        queryDirty={queryDirty}
        onAppSelect={handleAppSelect}
        onTimeChange={setTimeRange}
        onThemeChange={setTheme}
        onReset={handleReset}
      />
      <QueryBar
        query={queryInput}
        onQueryChange={setQueryInput}
        onRun={(q) => {
          const parsed = parseQuery(q);
          if (parsed) {
            query.execute(parsed.service, parsed.profileType, timeRange);
          }
        }}
      />

      {query.error && (
        <div className="app-error">
          <span>Unable to reach Pyroscope backend.</span>
          <span className="app-error-detail">{query.error}</span>
        </div>
      )}

      <div className="app-content">
        <Panel title={`${profileTypeRateLabel(profileType)}`}>
          <TimeSeries data={query.timeline} timeRange={timeRange} profileTypeId={profileType} />
        </Panel>

        <Panel
          title="Flamegraph"
          meta={`${service} · ${profileTypeLabel(profileType)} · ${timeRange}`}
        >
          <FlameGraph data={query.flamegraph} theme={theme} profileTypeId={profileType} />
        </Panel>
      </div>
    </div>
  );
}
