import { RequestNotOkError, request } from '@pyroscope/services/base';

export async function isMultiTenancyEnabled() {
  const res = await request('/querier.v1.QuerierService/LabelNames', {
    // Without this it would automatically add the OrgID
    // Which doesn't tell us whether multitenancy is enabled or not
    headers: {
      'X-Scope-OrgID': '',
      'content-type': 'application/json',
    },
    method: 'POST',
    body: JSON.stringify({
      matchers: ['{__profile_type__="app"}'],
    }),
  });

  // If everything went okay even without passing an OrgID, we can assume it's a non multitenant instance
  if (res.isOk) {
    return false;
  }

  return isOrgRequiredError(res);
}

function isOrgRequiredError(res: Awaited<ReturnType<typeof request>>) {
  // TODO: is 'no org id' a stable message?
  return (
    res.isErr &&
    res.error instanceof RequestNotOkError &&
    res.error.code === 401 &&
    res.error.description.includes('no org id')
  );
}
