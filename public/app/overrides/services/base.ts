import { Result } from '@webapp/util/fp';
import {
  type RequestError,
  mountRequest,
  RequestNotOkWithErrorsList,
  RequestNotOkError,
  RequestIncompleteError,
  ResponseOkNotInJSONFormat,
  ResponseNotOkInHTMLFormat,
  RequestAbortedError,
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

  return connectRequest(request, {
    ...config,
    headers,
  });
}

export async function connectRequest(
  request: RequestInfo,
  config?: RequestInit
): Promise<Result<unknown, RequestError>> {
  const req = mountRequest(request);
  let response: Response;
  try {
    response = await fetch(req, config);
  } catch (e) {
    // 'e' is unknown, but most cases it should be an Error
    let message = '';
    if (e instanceof Error) {
      message = e.message;
    }

    if (e instanceof Error && e.name === 'AbortError') {
      return Result.err(new RequestAbortedError(message));
    }

    return Result.err(new RequestIncompleteError(message));
  }

  if (!response.ok) {
    const textBody = await response.text();

    // There's nothing in the body, so let's use a default message
    if (!textBody || !textBody.length) {
      return Result.err(
        new RequestNotOkError(response.status, 'No description available')
      );
    }

    // We know there's data, so let's check if it's in JSON format
    try {
      const data = JSON.parse(textBody);

      // Check if it's 401 unauthorized error
      if (response.status === 401) {
        // TODO: Introduce some kind of interceptor (?)
        // if (!/\/(login|signup)$/.test(window?.location?.pathname)) {
        //   window.location.href = mountURL('/login');
        // }
        if ('message' in data && typeof data.message === 'string') {
          return Result.err(
            new RequestNotOkError(response.status, data.message)
          );
        }
        return Result.err(new RequestNotOkError(response.status, data.error));
      }

      // Usually it's a feedback on user's actions like form validation
      if ('errors' in data && Array.isArray(data.errors)) {
        return Result.err(
          new RequestNotOkWithErrorsList(response.status, data.errors)
        );
      }

      // Error message may come in an 'error' field
      if ('error' in data && typeof data.error === 'string') {
        return Result.err(new RequestNotOkError(response.status, data.error));
      }

      // Error message may come in an 'message' field
      if ('message' in data && typeof data.message === 'string') {
        return Result.err(new RequestNotOkError(response.status, data.message));
      }

      return Result.err(
        new RequestNotOkError(
          response.status,
          `Could not identify an error message. Payload is ${JSON.stringify(
            data
          )}`
        )
      );
    } catch (e) {
      // We couldn't parse, but there's definitly some data
      // We must handle this case since the go server sometimes responds with plain text

      // It's HTML
      // Which normally happens when hitting a broken URL, which makes the server return the SPA
      // Poor heuristic for identifying it's a html file
      if (/<\/?[a-z][\s\S]*>/i.test(textBody)) {
        return Result.err(
          new ResponseNotOkInHTMLFormat(response.status, textBody)
        );
      }
      return Result.err(new RequestNotOkError(response.status, textBody));
    }
  }

  // Server responded with 2xx
  const textBody = await response.text();

  // There's nothing in the body
  if (!textBody || !textBody.length) {
    return Result.ok({
      statusCode: response.status,
    });
  }

  // We know there's data, so let's check if it's in JSON format
  try {
    const data = JSON.parse(textBody);

    // We could parse the response
    return Result.ok(data);
  } catch (e) {
    // We couldn't parse, but there's definitly some data
    return Result.err(new ResponseOkNotInJSONFormat(response.status, textBody));
  }
}
