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
import { Annotation, AnnotationSchema } from '@webapp/models/annotation';
import type { RequestError } from './base';
import { request, parseResponse } from './base';

export interface RenderOutput {
  profile: Profile;
  timeline: Timeline;
  groups?: Groups;
  annotations: Annotation[];
}

// Default to empty array if not present
const defaultAnnotationsSchema = z.preprocess((a) => {
  if (!a) {
    return [];
  }
  return a;
}, z.array(AnnotationSchema));

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
    z.object({
      timeline: TimelineSchema,
      annotations: defaultAnnotationsSchema,
    })
  )
    .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
    .safeParse(response.value);

  if (parsed.success) {
    // TODO: strip timeline
    const profile = parsed.data;
    const { timeline, annotations } = parsed.data;

    return Result.ok({
      profile,
      timeline,
      annotations,
    });
  }

  return Result.err(parsed.error);
}

interface mergeWithQueryIDProps {
  queryID: string;
  refreshToken?: string;
  maxNodes: string | number;
}

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

export interface getHeatmapProps {
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
export interface HeatmapOutput {
  heatmap: Heatmap | null;
  profile?: Profile;
}

export async function getHeatmap(
  props: getHeatmapProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<HeatmapOutput, RequestError | ZodError>> {
  const params = new URLSearchParams({
    ...props,
    minValue: props.minValue.toString(),
    maxValue: props.maxValue.toString(),
    heatmapTimeBuckets: props.heatmapTimeBuckets.toString(),
    heatmapValueBuckets: props.heatmapValueBuckets.toString(),
  });

  const response = await request(`/api/exemplars:query?${params}`, {
    signal: controller?.signal,
  });

  if (response.isOk) {
    const parsed = FlamebearerProfileSchema.merge(
      z.object({ timeline: TimelineSchema })
    )
      .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
      .merge(z.object({ heatmap: HeatmapSchema.nullable() }))
      .safeParse(response.value);

    if (parsed.success) {
      const profile = parsed.data;
      const { heatmap } = parsed.data;

      if (heatmap !== null) {
        return Result.ok({
          heatmap,
          profile,
        });
      }

      return Result.ok({
        heatmap: null,
      });
    }

    return Result.err<HeatmapOutput, RequestError>(response.error);
  }

  return Result.err<HeatmapOutput, RequestError>(response.error);
}

export interface SelectionProfileOutput {
  selectionProfile: Profile;
}

export interface selectionProfileProps {
  from: string;
  until: string;
  query: string;
  selectionStartTime: number;
  selectionEndTime: number;
  selectionMinValue: number;
  selectionMaxValue: number;
  heatmapTimeBuckets: number;
  heatmapValueBuckets: number;
}

export async function getHeatmapSelectionProfile(
  props: selectionProfileProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<SelectionProfileOutput, RequestError | ZodError>> {
  const params = new URLSearchParams({
    ...props,
    selectionStartTime: props.selectionStartTime.toString(),
    selectionEndTime: props.selectionEndTime.toString(),
    selectionMinValue: props.selectionMinValue.toString(),
    selectionMaxValue: props.selectionMaxValue.toString(),
    heatmapTimeBuckets: props.heatmapTimeBuckets.toString(),
    heatmapValueBuckets: props.heatmapValueBuckets.toString(),
  });

  const response = await request(`/api/exemplars:query?${params}`, {
    signal: controller?.signal,
  });

  if (response.isOk) {
    const parsed = FlamebearerProfileSchema.merge(
      z.object({ timeline: TimelineSchema })
    )
      .merge(z.object({ telemetry: z.object({}).passthrough().optional() }))
      .safeParse(response.value);

    if (parsed.success) {
      return Result.ok({
        selectionProfile: parsed.data,
      });
    }

    return Result.err<SelectionProfileOutput, RequestError>(response.error);
  }

  return Result.err<SelectionProfileOutput, RequestError>(response.error);
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
    .merge(
      z.object({
        telemetry: z.object({}).passthrough().optional(),
        annotations: defaultAnnotationsSchema,
      })
    )
    .merge(
      z.object({
        groups: z.preprocess((groups) => {
          const groupNames = Object.keys(groups as Groups);

          return groupNames.length
            ? groupNames
                .filter((g) => !!g.trim())
                .reduce(
                  (acc, current) => ({
                    ...acc,
                    [current]: (groups as Groups)[current],
                  }),
                  {}
                )
            : groups;
        }, GroupsSchema),
      })
    )
    .safeParse(response.value);

  if (parsed.success) {
    const profile = parsed.data;
    const { groups, annotations } = parsed.data;

    return Result.ok({
      profile,
      groups,
      annotations,
    });
  }

  return Result.err(parsed.error);
}
