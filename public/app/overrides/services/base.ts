import { Result } from '@webapp/util/fp';
import {
  type RequestError,
  request as ogRequest,
} from '@pyroscope/webapp/javascript/services/base';
import { tenantIDFromStorage } from '@phlare/services/tenant';

export * from '@pyroscope/webapp/javascript/services/base';

/**
 * request wraps around the original request
 * while sending the OrgID if available
 */
export async function requestWithOrgID(
  request: RequestInfo,
  config?: RequestInit
): Promise<Result<unknown, RequestError>> {
  let headers = config?.headers;

  // Reuse headers if they were passed
  if (!config?.headers?.hasOwnProperty('X-Scope-OrgID')) {
    const tenantID = tenantIDFromStorage();
    headers = {
      ...config?.headers,
      ...(tenantID && { 'X-Scope-OrgID': tenantID }),
    };
  }

  return ogRequest(request, {
    ...config,
    headers,
  });
}
