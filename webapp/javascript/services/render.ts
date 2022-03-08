import { Result } from '@utils/fp';
import {
  Profile,
  FlamebearerProfileSchema,
  decodeFlamebearer,
} from '@pyroscope/models';
import type { ZodError } from 'zod';
import type { RequestError } from './base';
import { request } from './base';
import { buildRenderURL } from '../util/updateRequests';

export interface RenderOutput {
  raw: Profile;
  profile: Profile;
}

interface renderSingleProps {
  from: string;
  until: string;
  query: string;
  refreshToken?: string;
  maxNodes: string | number;
}
export async function renderSingle(
  props: renderSingleProps
): Promise<Result<RenderOutput, RequestError | ZodError>> {
  const url = buildRenderURL(props);
  // TODO
  const response = await request(`${url}}&format=json`);

  if (response.isErr) {
    return Result.err<RenderOutput, RequestError>(response.error);
  }

  const parsed = FlamebearerProfileSchema.safeParse(response.value);
  if (parsed.success) {
    const profile = parsed.data;
    // essentially duplicate the original flamegraph, since we will mutate it
    const raw = JSON.parse(JSON.stringify(profile));
    decodeFlamebearer(profile);

    return Result.ok({
      raw,
      profile,
    });
  }

  return Result.err(parsed.error);
}
