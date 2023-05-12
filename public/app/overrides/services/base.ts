import { Result } from '@webapp/util/fp';
import {
  type RequestError,
  request as ogRequest,
} from '../../../../node_modules/pyroscope-oss/webapp/javascript/services/base';
import { tenantIDFromStorage } from '@phlare/services/tenant';

export * from '../../../../node_modules/pyroscope-oss/webapp/javascript/services/base';

/**
 * request wraps around the original request
 * while sending the OrgID if available
 */
export async function request(
  request: RequestInfo,
  config?: RequestInit
): Promise<Result<unknown, RequestError>> {
  let headers = config?.headers;

  // Reuse headers if they were passed
  if (!config?.headers?.hasOwnProperty('X-Scope-OrgID')) {
    headers = {
      ...config?.headers,
      'X-Scope-OrgID': tenantIDFromStorage(),
    };
  }

  return ogRequest(request, {
    ...config,
    headers,
  });
}
