import { z, ZodError } from 'zod';
import { Result } from '@pyroscope/util/fp';
import { modelToResult } from '@pyroscope/models/utils';

const fooModel = z.array(z.string());

describe('modelToResult', () => {
  it('parses unkown object', () => {
    const got = modelToResult(fooModel, [] as unknown);
    expect(got).toMatchObject(Result.ok([]));
  });

  it('gives an error when object cant be parsed', () => {
    const got = modelToResult(fooModel, null);

    expect(got.isErr).toBe(true);

    // We don't care exactly about the error format, only that it's a ZodError
    expect((got as any).error instanceof ZodError).toBe(true);
  });
});
