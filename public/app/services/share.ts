/* eslint-disable import/prefer-default-export */
import { Result } from '@pyroscope/util/fp';
import type { ZodError } from 'zod';
import { Profile } from '@pyroscope/legacy/models';
import {
  FlamegraphDotComResponse,
  flamegraphDotComResponseScheme,
} from '@pyroscope/models/flamegraphDotComResponse';
import type { RequestError } from './base';
import { request, parseResponse } from './base';

interface ShareWithFlamegraphDotcomProps {
  flamebearer: Profile;
  name?: string;
  groupByTag?: string;
  groupByTagValue?: string;
}

export async function shareWithFlamegraphDotcom({
  flamebearer,
  name,
  groupByTag,
  groupByTagValue,
}: ShareWithFlamegraphDotcomProps): Promise<
  Result<FlamegraphDotComResponse, RequestError | ZodError>
> {
  const response = await request('/export', {
    method: 'POST',
    body: JSON.stringify({
      name,
      groupByTag,
      groupByTagValue,
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
