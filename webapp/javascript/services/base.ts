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

interface ResponseInvalidJSONError {
  message: string;
  data: any;
}

type RequestError =
  | RequestNotOkError
  | RequestIncompleteError
  | ResponseInvalidJSONError;

export async function get(
  path: string,
  config?: RequestInit
): Promise<Result<unknown, RequestError>> {
  let baseURL = basename();

  // There's no explicit baseURL configured
  // So let's try to infer one
  // This is useful for eg in tests
  if (!baseURL) {
    baseURL = window.location.href;
  }

  const address = new URL(path, baseURL).href;

  let response;
  try {
    response = await fetch(address, config);
  } catch (e) {
    // Fetch failed
    // https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API
    return Result.err({
      message: e.message,
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
