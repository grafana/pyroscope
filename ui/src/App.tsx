import { useState, useEffect } from 'react';
import './theme.css';
import './App.css';
import { NavBar } from '@components/NavBar';
import { FlameGraph } from '@components/FlameGraph';
import { QueryBar } from '@components/QueryBar';
import { TimeSeries } from '@components/TimeSeries';
import { Panel } from '@components/Panel';
import { TenantDialog } from '@components/TenantDialog';
import { usePyroscopeQuery, type ProfileType } from '@hooks/usePyroscopeQuery';
import { useTenant } from '@hooks/useTenant';
import {
  profileTypeLabel,
  profileTypeRateLabel,
  sortProfileTypes,
} from '@api/client';

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

function parseQuery(
  q: string,
): { service: string; profileType: string } | null {
  const service = q.match(/service_name\s*=\s*"([^"]+)"/)?.[1];
  const profileType = q.match(/profile_type\s*=\s*"([^"]+)"/)?.[1];
  if (!service || !profileType) return null;
  return { service, profileType };
}

export default function App() {
  const { theme, setTheme } = useTheme();
  const tenant = useTenant();
  const [service, setService] = useState('');
  const [profileType, setProfileType] = useState<ProfileType>('');
  const [timeRange, setTimeRange] = useState('now-1h');
  const [absoluteRange, setAbsoluteRange] = useState<
    | {
        start: number;
        end: number;
      }
    | undefined
  >(undefined);
  const [queryUserInput, setQueryUserInput] = useState<string | null>(null);
  const queryInput =
    queryUserInput ??
    (service || profileType ? buildQuery(service, profileType) : '');

  const query = usePyroscopeQuery({
    service,
    profileType,
    timeRange,
    absoluteRange,
    tenantID: tenant.tenantID,
  });

  useEffect(() => {
    if (query.servicesLoading || service) return;
    const first = query.services[0];
    if (!first) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setService(first.name);
    setProfileType(
      sortProfileTypes(first.profileTypes).find(
        (pt): pt is string => typeof pt === 'string',
      ) ?? '',
    );
  }, [query.services, query.servicesLoading, service]);

  const handleAppSelect = (s: string, pt: ProfileType) => {
    setService(s);
    setProfileType(pt);
    setQueryUserInput(null);
  };

  const queryDirty =
    !!service && queryInput !== buildQuery(service, profileType);
  const handleReset = () => setQueryUserInput(null);

  if (tenant.status === 'loading') return null;

  if (tenant.status === 'needs_tenant_id') {
    return <TenantDialog onSaved={tenant.setTenantID} />;
  }

  const isMultiTenant = tenant.status === 'multi_tenant';

  return (
    <div className="app">
      {tenant.wantsToChange && (
        <TenantDialog
          currentTenantID={tenant.tenantID}
          onSaved={tenant.setTenantID}
        />
      )}
      <NavBar
        services={query.services}
        servicesLoading={query.servicesLoading}
        service={service}
        profileType={profileType}
        timeRange={timeRange}
        theme={theme}
        queryDirty={queryDirty}
        onAppSelect={handleAppSelect}
        absoluteRange={absoluteRange}
        onTimeChange={(v) => {
          setAbsoluteRange(undefined);
          setTimeRange(v);
        }}
        onThemeChange={setTheme}
        onReset={handleReset}
        isMultiTenant={isMultiTenant}
        tenantID={tenant.tenantID}
        onChangeTenant={() => tenant.setWantsToChange(true)}
      />
      <QueryBar
        query={queryInput}
        onQueryChange={setQueryUserInput}
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
          <TimeSeries
            data={query.timeline}
            timeRange={timeRange}
            profileTypeId={profileType}
            startMs={absoluteRange?.start}
            endMs={absoluteRange?.end}
            onRangeSelect={(start, end) => setAbsoluteRange({ start, end })}
          />
        </Panel>

        <Panel
          title="Flamegraph"
          meta={`${service} · ${profileTypeLabel(profileType)} · ${timeRange}`}
        >
          <FlameGraph
            data={query.flamegraph}
            theme={theme}
            profileTypeId={profileType}
          />
        </Panel>
      </div>
    </div>
  );
}
