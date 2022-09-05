import { z, ZodError } from 'zod';

import { Result } from '@webapp/util/fp';
import { RequestError, request } from './base';

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

interface ExemplarsProps {
  startTime?: string;
  endTime?: string;
  minValue?: string;
  maxValue?: string;
  query: string;
  maxNodes?: string;
  heatmapTimeBuckets?: string;
  heatmapValueBuckets?: string;
}

export type Heatmap = z.infer<typeof HeatmapSchema>;

interface Response {
  heatmap: Heatmap;
}

export async function getExemplars(
  props: ExemplarsProps,
  controller?: {
    signal?: AbortSignal;
  }
): Promise<Result<Response, RequestError | ZodError>> {
  const queryString = Object.entries(props).reduce(
    (acc, [key, value]) => acc + (acc ? `&${key}=${value}` : `${key}=${value}`),
    ''
  );
  const response = await request(`/api/exemplars:query?${queryString}`, {
    signal: controller?.signal,
  });

  if (response.isOk) {
    const parsed = z
      .object({ heatmap: HeatmapSchema })
      .safeParse(response.value);

    if (parsed.success) {
      return Result.ok({
        heatmap: parsed.data.heatmap,
      });
    }

    return Result.err<Response, RequestError>(response.error);
  }

  return Result.err<Response, RequestError>(response.error);
}
