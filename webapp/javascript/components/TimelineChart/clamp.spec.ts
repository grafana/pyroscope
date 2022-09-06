import clamp from './clamp';

describe('clamp', () => {
  it('value is less than min', () => {
    expect(clamp(1, 3, 0)).toEqual(1);
  });
  it('value is greater than min', () => {
    expect(clamp(1, 3, 4)).toEqual(3);
  });
  it('value is in limits', () => {
    expect(clamp(1, 3, 2)).toEqual(2);
  });
});
