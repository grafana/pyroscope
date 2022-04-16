/* eslint-disable import/prefer-default-export */
import { Result } from '@webapp/util/fp';
import type { ZodError } from 'zod';
import { Profile } from '@pyroscope/models';
import {
  FlamegraphDotComResponse,
  flamegraphDotComResponseScheme,
} from '@webapp/models/flamegraphDotComResponse';
import type { RequestError } from './base';
import { request, parseResponse } from './base';

interface shareWithFlamegraphDotcomProps {
  flamebearer: Profile;
  name?: string;
}

export async function shareWithFlamegraphDotcom({
  flamebearer,
  name,
}: shareWithFlamegraphDotcomProps): Promise<
  Result<FlamegraphDotComResponse, RequestError | ZodError>
> {
  const response = await request('/export', {
    method: 'POST',
    body: JSON.stringify({
      name,
      // TODO:
      // use buf.toString
      profile: btoa(JSON.stringify(flamebearer)),
      type: 'application/json',
    }),
  });

  if (response.isOk) {
    return parseResponse(response, flamegraphDotComResponseScheme);
  }

  return Result.err<FlamegraphDotComResponse, RequestError>(response.error);
}
