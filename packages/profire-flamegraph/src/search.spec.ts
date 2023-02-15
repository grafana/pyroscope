import { isMatch } from './search';

describe('search', () => {
  it('matches substrings', () => {
    expect(isMatch('foo', 'foobar')).toBe(true);
  });

  it('ignores cases', () => {
    expect(isMatch('foo', 'FOOBAR')).toBe(true);
  });
});
