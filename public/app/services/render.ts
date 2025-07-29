import { Result } from '@pyroscope/util/fp';
import {
  Profile,
  Groups,
  FlamebearerProfileSchema,
} from '@pyroscope/legacy/models';
import { z } from 'zod';
import type { ZodError } from 'zod';
import { buildRenderURL } from '@pyroscope/util/updateRequests';
import { Timeline, TimelineSchema } from '@pyroscope/models/timeline';
import type { RequestError } from '@pyroscope/services/base';
import { request } from '@pyroscope/services/base';

export interface RenderOutput {
  profile: Profile;
  timeline: Timeline;
  groups?: Groups;
}

interface RenderSingleProps {
  from: string;
  until: string;
  query: string;
  refreshToken?: string;
  maxNodes: string | number;
}
export async function renderSingle(
  props: RenderSingleProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<RenderOutput, RequestError | ZodError>> {
  const url = buildRenderURL(props);
  // TODO
  const response = await request(`/pyroscope${url}&format=json`, {
    signal: controller?.signal,
  });

  if (response.isErr) {
    return Result.err<RenderOutput, RequestError>(response.error);
  }

  const parsed = FlamebearerProfileSchema.merge(
    z.object({
      timeline: TimelineSchema,
    })
  )
    .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
    .safeParse(response.value);

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
