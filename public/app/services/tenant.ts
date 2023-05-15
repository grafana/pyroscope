import { RequestNotOkError, requestWithOrgID } from '@webapp/services/base';
import store from '@webapp/redux/store';

export async function isMultiTenancyEnabled() {
  const res = await requestWithOrgID('/pyroscope/label-values?label=__name__', {
    // Without this it would automatically add the OrgID
    // Which doesn't tell us whether multitenancy is enabled or not
    headers: {
      'X-Scope-OrgID': '',
    },
  });

  // If everything went okay even without passing an OrgID, we can assume it's a non multitenant instance
  if (res.isOk) {
    return false;
  }

  return isOrgRequiredError(res);
}

function isOrgRequiredError(res: Awaited<ReturnType<typeof requestWithOrgID>>) {
  // TODO: is 'no org id' a stable message?
  return (
    res.isErr &&
    res.error instanceof RequestNotOkError &&
    res.error.code === 401 &&
    res.error.description === 'no org id\n'
  );
}

export function tenantIDFromStorage(): string {
  return store.getState().tenant.tenantID || '';
}
