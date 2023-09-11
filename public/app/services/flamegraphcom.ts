import { parseResponse, RequestNotOkError } from '@pyroscope/services/base';
import { z, ZodError } from 'zod';
import { Result } from '@pyroscope/util/fp';
import type { RequestError } from '@pyroscope/services/base';
import { Profile } from '@pyroscope/legacy/models';

export async function flameGraphUpload(
  name: string,
  flamebearer: Profile
): Promise<Result<string, RequestError | ZodError>> {
  const response = await fetch('https://flamegraph.com/api/upload/v1', {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
    },
    body: JSON.stringify({
      fileTypeData: {
        units: flamebearer.metadata.units,
        spyName: flamebearer.metadata.spyName,
      },
      name,
      profile: btoa(JSON.stringify(flamebearer)),
      type: 'json',
    }),
  });
  if (!response.ok) {
    return Result.err(
      new RequestNotOkError(
        response.status,
        `Failed to upload to flamegraph.com: ${response.statusText}`
      )
    );
  }
  const body = await response.text();
  return parseResponse(
    Result.ok(JSON.parse(body)),
    z
      .preprocess(
        (arg) => {
          return arg;
        },
        z.object({
          key: z.string(),
          url: z.string(),
        })
      )
      .transform((arg) => arg.url)
  );
}
