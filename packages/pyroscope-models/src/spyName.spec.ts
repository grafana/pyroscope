import { SpyNameSchema, AllSpyNames } from './spyName';

describe('SpyName', () => {
  it('fails when when passing an unsupported value', () => {
    expect(SpyNameSchema.safeParse('foo').success).toBe(false);
  });

  it('defaults to "unknown" when absent', () => {
    expect(SpyNameSchema.parse('')).toBe('unknown');
  });

  test.each(AllSpyNames.map((a) => [a, a]))(
    'parse("%s") should return "%s"',
    (spyName) => {
      expect(SpyNameSchema.parse(spyName)).toBe(spyName);
    }
  );
});
