import { Result } from '@pyroscope/util/fp';
import {
  Profile,
  Groups,
  FlamebearerProfileSchema,
  GroupsSchema,
} from '@pyroscope/legacy/models';
import { z } from 'zod';
import type { ZodError } from 'zod';
import {
  buildRenderURL,
  buildMergeURLWithQueryID,
} from '@pyroscope/util/updateRequests';
import { Timeline, TimelineSchema } from '@pyroscope/models/timeline';
import { Annotation, AnnotationSchema } from '@pyroscope/models/annotation';
import type { RequestError } from '@pyroscope/services/base';
import { request, parseResponse } from '@pyroscope/services/base';

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

export type RenderDiffResponse = z.infer<typeof FlamebearerProfileSchema>;

interface RenderDiffProps {
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
  props: RenderDiffProps,
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
  });

  const response = await request(`/pyroscope/render-diff?${params}`, {
    signal: controller?.signal,
  });

  return parseResponse<z.infer<typeof FlamebearerProfileSchema>>(
    response,
    FlamebearerProfileSchema
  );
}

const RenderExploreSchema = FlamebearerProfileSchema.extend({
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
}).transform((values) => {
  return {
    profile: values,
    groups: values.groups,
  };
});

interface RenderExploreProps extends Omit<RenderSingleProps, 'maxNodes'> {
  groupBy: string;
  grouByTagValue: string;
}

export type RenderExploreOutput = z.infer<typeof RenderExploreSchema>;

export async function renderExplore(
  props: RenderExploreProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<RenderExploreOutput, RequestError | ZodError>> {
  const url = buildRenderURL(props);
  const response = await request(`/pyroscope/${url}&format=json`, {
    signal: controller?.signal,
  });
  return parseResponse<RenderExploreOutput>(response, RenderExploreSchema);
}

interface MergeWithQueryIDProps {
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

export interface MergeOutput {
  profile: Profile;
  mergeMetadata: MergeMetadata;
}

const MergeMetadataSchema = z.object({
  appName: z.string(),
  startTime: z.string(),
  endTime: z.string(),
  profilesLength: z.number(),
});

export async function mergeWithQueryID(
  props: MergeWithQueryIDProps,
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

export interface GetHeatmapProps {
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
  props: GetHeatmapProps,
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

export interface SelectionProfileProps {
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
  props: SelectionProfileProps,
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
