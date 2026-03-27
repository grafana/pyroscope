import { useState, useEffect } from 'react';
import { checkMultitenancy, setOrgID } from '@api/client';

const STORAGE_KEY = 'pyroscope:tenantID';

export type TenantStatus =
  | 'loading'
  | 'single_tenant'
  | 'needs_tenant_id'
  | 'multi_tenant';

export function useTenant() {
  const [status, setStatus] = useState<TenantStatus>('loading');
  const [tenantID, setTenantIDState] = useState<string | undefined>(undefined);
  const [wantsToChange, setWantsToChange] = useState(false);

  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY) ?? undefined;

    // Set orgID synchronously so API calls made by sibling effects in the same
    // render cycle (e.g. usePyroscopeQuery's fetchServices) use the correct tenant.
    if (saved) setOrgID(saved);

    checkMultitenancy().then((result) => {
      if (result === 'single_tenant' || result === 'error') {
        setOrgID('');
        setStatus('single_tenant');
        return;
      }
      if (saved) {
        setTenantIDState(saved);
        setStatus('multi_tenant');
      } else {
        setStatus('needs_tenant_id');
      }
    });
  }, []);

  const setTenantID = (id: string) => {
    localStorage.setItem(STORAGE_KEY, id);
    setOrgID(id);
    setTenantIDState(id);
    setStatus('multi_tenant');
    setWantsToChange(false);
  };

  return { status, tenantID, setTenantID, wantsToChange, setWantsToChange };
}
