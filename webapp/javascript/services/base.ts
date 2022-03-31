/* eslint-disable max-classes-per-file */
/* eslint-disable import/prefer-default-export */
import { Result } from '@webapp/util/fp';
import type { ZodError } from 'zod';
import { modelToResult } from '@webapp/models/utils';
import { CustomError } from 'ts-custom-error';
import basename from '@webapp/util/baseurl';

// RequestNotOkError refers to when the Response is not within the 2xx range
export class RequestNotOkError extends CustomError {
  public constructor(public code: number, public description: string) {
    super(
      `Request failed with statusCode: '${code}' and description: '${description}'`
    );
  }
}

// RequestError refers to when the request is not completed
// For example CORS errors or timeouts
// or simply the address is wrong
export class RequestIncompleteError extends CustomError {
  public constructor(public description: string) {
    super(`Request failed to be completed. Description: '${description}'`);
  }
}

// When the server returns a list of errors
export class RequestNotOkWithErrorsList extends CustomError {
  public constructor(public code: number, public errors: string[]) {
    super(`Server returned with multiple errors: ${errors.join(', ')}`);
  }
}

export class ResponseOkNotInJSONFormat extends CustomError {
  public constructor(public code: number, public body: string) {
    super(
      `Server returned with code: '${code}'. The body that could not be parsed contains '${body}'`
    );
  }
}

export type RequestError =
  | RequestNotOkError
  | RequestNotOkWithErrorsList
  | RequestIncompleteError
  | ResponseOkNotInJSONFormat;

function mountURL(req: RequestInfo): string {
  const baseName = basename();

  if (baseName) {
    if (typeof req === 'string') {
      return new URL(`${baseName}/${req}`, window.location.href).href;
    }

    // req is an object
    return new URL(`${baseName}/${req.url}`, window.location.href).href;
  }

  // no basename
  if (typeof req === 'string') {
    return new URL(`${req}`, window.location.href).href;
  }
  return new URL(`${req}`, window.location.href).href;
}

export function mountRequest(req: RequestInfo): RequestInfo {
  const url = mountURL(req);

  if (typeof req === 'string') {
    return url;
  }

  return {
    ...req,
    url: new URL(req.url, url).href,
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
    let message = '';
    if (e instanceof Error) {
      message = e.message;
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
        if (window && window.location) {
          window.location.href = '/login';
        }
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

// We have to call it something else otherwise it will conflict with the global "Response"
type ResponseFromRequest = Awaited<ReturnType<typeof request>>;
type Schema = Parameters<typeof modelToResult>[0];

// parseResponse parses a response with given schema if the request has not failed
export function parseResponse<T>(
  res: ResponseFromRequest,
  schema: Schema
): Result<T, RequestError | ZodError> {
  if (res.isErr) {
    return Result.err<T, RequestError>(res.error);
  }

  return modelToResult(schema, res.value) as Result<T, ZodError<ShamefulAny>>;
}
