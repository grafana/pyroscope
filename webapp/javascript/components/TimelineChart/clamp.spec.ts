import clamp from './clamp';

describe('clamp', () => {
  it('value is less than min', () => {
    expect(clamp(1, 0, 3)).toEqual(1);
  });
  it('value is greater than min', () => {
    expect(clamp(1, 4, 3)).toEqual(3);
  });
  it('value is in limits', () => {
    expect(clamp(1, 2, 3)).toEqual(2);
  });
});
