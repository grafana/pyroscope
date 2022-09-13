import { randomId } from './randomId';

describe('randomID', () => {
  it('generates a randomID with 5 characters', () => {
    expect(randomId().length).toBe(5);
  });
});
