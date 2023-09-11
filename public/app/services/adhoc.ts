import { Result } from '@pyroscope/util/fp';
import { CustomError } from 'ts-custom-error';
import { FlamebearerProfileSchema, Profile } from '@pyroscope/legacy/models';
import { AllProfilesSchema, AllProfiles } from '@pyroscope/models/adhoc';
import type { ZodError } from 'zod';
import { z } from 'zod';
import { request, parseResponse } from './base';
import type { RequestError } from './base';

const uploadResponseSchema = z.object({
  flamebearer: FlamebearerProfileSchema,
  id: z.string().min(1),
});
type UploadResponse = z.infer<typeof uploadResponseSchema>;

export async function upload(
  file: File,
  fileTypeData?: { spyName: string; units: string }
): Promise<
  Result<UploadResponse, FileToBase64Error | RequestError | ZodError>
> {
  // prepare body
  const b64 = await fileToBase64(file);
  if (b64.isErr) {
    return Result.err<UploadResponse, FileToBase64Error>(b64.error);
  }

  const response = await request('/api/adhoc/v1/upload', {
    method: 'POST',
    body: JSON.stringify({
      filename: file.name,
      profile: b64.value,
      fileTypeData: fileTypeData || undefined,
    }),
  });
  return parseResponse(response, uploadResponseSchema);
}

export async function retrieve(
  id: string
): Promise<Result<Profile, RequestError | ZodError>> {
  const response = await request(`/api/adhoc/v1/profile/${id}`);
  return parseResponse<Profile>(response, FlamebearerProfileSchema);
}

export async function retrieveDiff(
  leftId: string,
  rightId: string
): Promise<Result<Profile, RequestError | ZodError>> {
  const response = await request(`/api/adhoc/v1/diff/${leftId}/${rightId}`);
  return parseResponse<Profile>(response, FlamebearerProfileSchema);
}

export async function retrieveAll(): Promise<
  Result<AllProfiles, RequestError | ZodError>
> {
  const response = await request(`/api/adhoc/v1/profiles`);
  return parseResponse(response, AllProfilesSchema);
}

/**
 * represents an error when trying to convert a File to base64
 */
export class FileToBase64Error extends CustomError {
  constructor(
    public filename: string,
    public message: string,
    public cause?: Error | DOMException
  ) {
    super(message);
  }
}

export default function fileToBase64(
  file: File
): Promise<Result<string, FileToBase64Error>> {
  return new Promise((resolve) => {
    const reader = new FileReader();

    reader.onloadend = () => {
      // this is always called, even on failures
      if (!reader.error) {
        if (!reader.result) {
          return resolve(
            Result.err(new FileToBase64Error(file.name, 'No result'))
          );
        }

        // reader can be used with 'readAsArrayBuffer' which returns an ArrayBuffer
        // therefore for the sake of the compiler we must check its value
        if (typeof reader.result === 'string') {
          // remove the prefix
          const base64result = reader.result.split(';base64,')[1];
          if (!base64result) {
            return resolve(
              Result.err(
                new FileToBase64Error(file.name, 'Failed to strip prefix')
              )
            );
          }

          // split didn't work
          if (base64result === reader.result) {
            return resolve(
              Result.err(
                new FileToBase64Error(file.name, 'Failed to strip prefix')
              )
            );
          }

          // the string is prefixed with
          return resolve(Result.ok(base64result));
        }
      }

      // should not happen
      return resolve(Result.err(new FileToBase64Error(file.name, 'No result')));
    };

    reader.onerror = () => {
      resolve(
        Result.err(
          new FileToBase64Error(
            file.name,
            'File reading has failed',
            reader.error || undefined
          )
        )
      );
    };

    reader.readAsDataURL(file);
  });
}
