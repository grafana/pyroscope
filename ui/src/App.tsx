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
import { profileTypeRateLabel, sortProfileTypes } from '@api/client';

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

function buildQuery(service: string): string {
  return `{service_name="${service}"}`;
}
function parseQueryServiceName(q: string): string | null {
  const service = q.match(/service_name\s*=\s*"([^"]+)"/)?.[1];
  if (!service) return null;
  return service;
}
export default function App() {
  const { theme, setTheme } = useTheme();
  const tenant = useTenant();
  const [profileType, setProfileType] = useState<ProfileType>('');
  const [timeRange, setTimeRange] = useState('now-1h');
  const [absoluteRange, setAbsoluteRange] = useState<
    | {
        start: number;
        end: number;
      }
    | undefined
  >(undefined);
  const [queryUserInput, setQueryUserInput] = useState<string>('');
  const [labelSelector, setLabelSelector] = useState<string>('');
  const service = parseQueryServiceName(labelSelector);

  const query = usePyroscopeQuery({
    labelSelector,
    profileType,
    timeRange,
    absoluteRange,
    tenantID: tenant.tenantID,
  });

  useEffect(() => {
    if (query.servicesLoading || labelSelector) return;
    const first = query.services[0];
    if (!first) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setQueryUserInput(buildQuery(first.name));
    setLabelSelector(buildQuery(first.name));
    setProfileType(
      sortProfileTypes(first.profileTypes).find(
        (pt): pt is string => typeof pt === 'string',
      ) ?? '',
    );
  }, [query.services, query.servicesLoading, labelSelector]);

  const handleAppSelect = (s: string, pt: ProfileType) => {
    setQueryUserInput(buildQuery(s));
    setLabelSelector(buildQuery(s));
    setProfileType(pt);
  };

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
        service={service ?? ''}
        profileType={profileType}
        timeRange={timeRange}
        theme={theme}
        onAppSelect={handleAppSelect}
        absoluteRange={absoluteRange}
        onTimeChange={(v) => {
          setAbsoluteRange(undefined);
          setTimeRange(v);
        }}
        onThemeChange={setTheme}
        isMultiTenant={isMultiTenant}
        tenantID={tenant.tenantID}
        onChangeTenant={() => tenant.setWantsToChange(true)}
      />
      <QueryBar
        query={queryUserInput}
        onQueryChange={setQueryUserInput}
        onRun={(q) => {
          setLabelSelector(q);
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

        <Panel title="Flamegraph">
          <FlameGraph data={query.flamegraph} profileTypeId={profileType} />
        </Panel>
      </div>
    </div>
  );
}
