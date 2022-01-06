/* eslint-disable import/prefer-default-export */
import { Result } from '@utils/fp';
import basename from '../util/baseurl';

// RequestNotOkError refers to when the Response is not within the 2xx range
interface RequestNotOkError {
  statusCode: number;
  message: string;
}

// RequestError refers to when the request is not completed
// For example CORS errors or timeouts
interface RequestIncompleteError {
  message: string;
}

// ResponseInvalidJSONError refers to when the response is not a valid JSON
interface ResponseInvalidJSONError {
  message: string;
  data: any;
}

export type RequestError =
  | RequestNotOkError
  | RequestIncompleteError
  | ResponseInvalidJSONError;

function mountRequest(req: RequestInfo): RequestInfo {
  let baseURL = basename();

  // There's no explicit baseURL configured
  // So let's try to infer one
  // This is useful for eg in tests
  if (!baseURL) {
    baseURL = window.location.href;
  }

  // TODO:
  // figure out if there's already a base URL in the request
  if (typeof req === 'string') {
    return new URL(req, baseURL).href;
  }

  return {
    ...req,
    url: new URL(req.url, baseURL).href,
  };
}

export async function request(
  request: RequestInfo,
  config?: RequestInit
): Promise<Result<unknown, RequestError>> {
  const req = mountRequest(request);
  let response: Response;
  try {
    response = await fetch(req, config);
  } catch (e) {
    // 'e' is unknown, but most cases it should be an Error
    let message = 'Server failed to respond;';
    if (e instanceof Error) {
      message = e.message;
    }

    return Result.err({
      message,
    });
  }

  if (!response.ok) {
    const textBody = await response.text();

    // There's nothing in the body, so let's use a default message
    if (!textBody || !textBody.length) {
      return Result.err({
        statusCode: response.status,
        message: 'Request failed',
      });
    }

    // We know there's data, so let's check if it's in JSON format
    try {
      const data = JSON.parse(textBody);

      // We could parse the response
      return Result.err({
        statusCode: response.status,
        ...data,
      });
    } catch (e) {
      // We couldn't parse, but there's definitly some data
      // We must handle this case since the go server sometimes responds with plain text
      return Result.err({
        statusCode: response.status,
        message: textBody,
      });
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
    return Result.ok({
      statusCode: response.status,
      ...data,
    });
  } catch (e) {
    // We couldn't parse, but there's definitly some data
    return Result.err({
      statusCode: response.status,
      message: 'Failed to parse JSON',
      data: textBody,
    });
  }
}
