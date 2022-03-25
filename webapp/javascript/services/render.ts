import { Result } from '@webapp/util/fp';
import { Profile, FlamebearerProfileSchema } from '@pyroscope/models';
import { z } from 'zod';
import type { ZodError } from 'zod';
import {
  buildRenderURL,
  buildDiffRenderURL,
} from '@webapp/util/updateRequests';
import { Timeline, TimelineSchema } from '@webapp/models/timeline';
import type { RequestError } from './base';
import { request } from './base';

export interface RenderOutput {
  profile: Profile;
  timeline: Timeline;
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

  const parsed = FlamebearerProfileSchema.merge(
    z.object({ timeline: TimelineSchema })
  ).safeParse(response.value);

  if (parsed.success) {
    // TODO: strip timeline
    const profile = parsed.data;
    const { timeline } = parsed.data;

    return Result.ok({
      profile,
      timeline,
    });
  }

  return Result.err(parsed.error);
}

interface renderDiffProps {
  from: string;
  until: string;
  query: string;
  refreshToken?: string;
  maxNodes: string;
  leftFrom: string;
  leftUntil: string;
  rightFrom: string;
  rightUntil: string;
}
export async function renderDiff(
  props: renderDiffProps
): Promise<Result<RenderOutput, RequestError | ZodError>> {
  const url = buildDiffRenderURL(props);
  // TODO
  const response = await request(`${url}}&format=json`);

  if (response.isErr) {
    return Result.err<RenderOutput, RequestError>(response.error);
  }

  const parsed = FlamebearerProfileSchema.merge(
    z.object({ timeline: TimelineSchema })
  ).safeParse(response.value);

  if (parsed.success) {
    // TODO: strip timeline
    const profile = parsed.data;
    const { timeline } = parsed.data;

    return Result.ok({
      profile,
      timeline,
    });
  }

  return Result.err(parsed.error);
}
