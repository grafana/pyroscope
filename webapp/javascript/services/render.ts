import { Result } from '@webapp/util/fp';
import {
  Profile,
  Groups,
  FlamebearerProfileSchema,
  GroupsSchema,
} from '@pyroscope/models/src';
import { z } from 'zod';
import type { ZodError } from 'zod';
import {
  buildRenderURL,
  buildMergeURLWithQueryID,
} from '@webapp/util/updateRequests';
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

interface mergeWithQueryIDProps {
  queryID: string;
  refreshToken?: string;
  maxNodes: string | number;
}

// z.infer<typeof MergeMetadataSchema> ?
interface MergeMetadata {
  appName: string;
  startTime: string;
  endTime: string;
  profilesLength: number;
}

const MergeMetadataSchema = z.object({
  appName: z.string(),
  startTime: z.string(),
  endTime: z.string(),
  profilesLength: z.number(),
});

export interface MergeOutput {
  profile: Profile;
  mergeMetadata: MergeMetadata;
}

export async function mergeWithQueryID(
  props: mergeWithQueryIDProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<MergeOutput, RequestError | ZodError>> {
  const url = buildMergeURLWithQueryID(props);
  // TODO
  const response = await request(`${url}&format=json`, {
    signal: controller?.signal,
  });

  if (response.isErr) {
    return Result.err<MergeOutput, RequestError>(response.error);
  }

  const parsed = FlamebearerProfileSchema.merge(
    z.object({ timeline: TimelineSchema })
  )
    .merge(z.object({ mergeMetadata: MergeMetadataSchema }))
    .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
    .safeParse(response.value);

  if (parsed.success) {
    // TODO: strip timeline
    const profile = parsed.data;
    const { mergeMetadata } = parsed.data;

    return Result.ok({
      profile,
      mergeMetadata,
    });
  }

  return Result.err(parsed.error);
}

const HeatmapSchema = z.object({
  startTime: z.number(),
  endTime: z.number(),
  minValue: z.number(),
  maxValue: z.number(),
  minDepth: z.number(),
  maxDepth: z.number(),
  timeBuckets: z.number(),
  valueBuckets: z.number(),
  values: z.array(z.array(z.number())),
});

export interface getExemplarsProps {
  query: string;
  from: string;
  until: string;
  minValue: number;
  maxValue: number;
  heatmapTimeBuckets: number;
  heatmapValueBuckets: number;
  maxNodes?: string;
}

export type Heatmap = z.infer<typeof HeatmapSchema>;
export interface ExemplarsOutput {
  heatmap: Heatmap;
}

export async function getExemplars(
  props: getExemplarsProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<ExemplarsOutput, RequestError | ZodError>> {
  const queryString = Object.entries(props).reduce(
    (acc, [key, value]) => acc + (acc ? `&${key}=${value}` : `${key}=${value}`),
    ''
  );
  const response = await request(`/api/exemplars:query?${queryString}`, {
    signal: controller?.signal,
  });

  if (response.isOk) {
    const parsed = FlamebearerProfileSchema.merge(
      z.object({ timeline: TimelineSchema })
    )
      .merge(z.object({ heatmap: HeatmapSchema }))
      .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
      .safeParse(response.value);

    if (parsed.success) {
      return Result.ok({
        heatmap: parsed.data.heatmap,
      });
    }

    return Result.err<ExemplarsOutput, RequestError>(response.error);
  }

  return Result.err<ExemplarsOutput, RequestError>(response.error);
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
