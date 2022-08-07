import { Result } from '@webapp/util/fp';
import {
  Profile,
  Groups,
  FlamebearerProfileSchema,
  GroupsSchema,
} from '@pyroscope/models/src';
import { z } from 'zod';
import type { ZodError } from 'zod';
import { buildRenderURL } from '@webapp/util/updateRequests';
import { Timeline, TimelineSchema } from '@webapp/models/timeline';
import type { RequestError } from './base';
import { request, parseResponse } from './base';

export interface RenderOutput {
  profile: Profile;
  timeline: Timeline;
  groups?: Groups;
}

interface renderSingleProps {
  from: string;
  until: string;
  query: string;
  refreshToken?: string;
  maxNodes: string | number;
}
export async function renderSingle(
  props: renderSingleProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<RenderOutput, RequestError | ZodError>> {
  const url = buildRenderURL(props);
  // TODO
  const response = await request(`${url}&format=json`, {
    signal: controller?.signal,
  });

  if (response.isErr) {
    return Result.err<RenderOutput, RequestError>(response.error);
  }

  const parsed = FlamebearerProfileSchema.merge(
    z.object({ timeline: TimelineSchema })
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

export type RenderDiffResponse = z.infer<typeof FlamebearerProfileSchema>;

interface renderDiffProps {
  leftFrom: string;
  leftUntil: string;
  leftQuery: string;
  refreshToken?: string;
  maxNodes: string;
  rightQuery: string;
  rightFrom: string;
  rightUntil: string;
}
export async function renderDiff(
  props: renderDiffProps,
  controller?: {
    signal?: AbortSignal;
  }
) {
  const params = new URLSearchParams({
    leftQuery: props.leftQuery,
    leftFrom: props.leftFrom,
    leftUntil: props.leftUntil,
    rightQuery: props.rightQuery,
    rightFrom: props.rightFrom,
    rightUntil: props.rightUntil,
    format: 'json',
  });

  const response = await request(`/render-diff?${params}`, {
    signal: controller?.signal,
  });

  return parseResponse<z.infer<typeof FlamebearerProfileSchema>>(
    response,
    FlamebearerProfileSchema
  );
}

interface renderExploreProps extends Omit<renderSingleProps, 'maxNodes'> {
  groupBy: string;
  grouByTagValue: string;
}

export interface RenderExploreOutput {
  profile: Profile;
  groups: Groups;
}

export async function renderExplore(
  props: renderExploreProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<RenderExploreOutput, RequestError | ZodError>> {
  const url = buildRenderURL(props);

  const response = await request(`${url}&format=json`, {
    signal: controller?.signal,
  });

  if (response.isErr) {
    return Result.err<RenderExploreOutput, RequestError>(response.error);
  }

  const parsed = FlamebearerProfileSchema.merge(
    z.object({ timeline: TimelineSchema })
  )
    .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
    .merge(z.object({ groups: GroupsSchema }))
    .safeParse(response.value);

  if (parsed.success) {
    const profile = parsed.data;
    const { groups } = parsed.data;

    return Result.ok({
      profile,
      groups,
    });
  }

  return Result.err(parsed.error);
}
