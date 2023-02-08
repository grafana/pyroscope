import { isMatch } from './search';

describe('search', () => {
  it('matches substrings', () => {
    expect(isMatch('foo', 'foobar')).toBe(true);
  });

  it('ignores cases', () => {
    expect(isMatch('foo', 'FOOBAR')).toBe(true);
  });

  it('accepts regex', () => {
    expect(isMatch('bar|foo', 'FOOBAR')).toBe(true);
  });

  it('accepts exact regex', () => {
    expect(isMatch('^foobar$', 'FOOBAR')).toBe(true);
    expect(isMatch('^foobar$', 'FOOBAR1')).toBe(false);
  });
});
