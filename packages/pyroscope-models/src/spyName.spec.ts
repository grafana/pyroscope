import { SpyNameSchema, AllSpyNames } from './spyName';

describe('SpyName', () => {
  it('defaults to "unknown" when absent', () => {
    expect(SpyNameSchema.parse('')).toBe('unknown');
  });

  it('defaults to "unknown" when value is not in the allowlist', () => {
    expect(SpyNameSchema.parse('other')).toBe('unknown');
  });

  test.each(AllSpyNames.map((a) => [a, a]))(
    'parse("%s") should return "%s"',
    (spyName) => {
      expect(SpyNameSchema.parse(spyName)).toBe(spyName);
    }
  );
});
