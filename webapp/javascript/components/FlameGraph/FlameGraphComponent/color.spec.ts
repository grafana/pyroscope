import {
  //  colorBasedOnDiff,
  colorBasedOnDiffPercent,
  colorFromPercentage,
} from './color';

describe.each([
  [100, 'rgba(200, 0, 0, 0.8)', 'full red'],
  [200, 'rgba(200, 0, 0, 0.8)', 'full red capped'],
  [50, 'rgba(200, 90, 90, 0.8)', 'half-way red'],

  [-100, 'rgba(0, 200, 0, 0.8)', 'full red'],
  [-200, 'rgba(0, 200, 0, 0.8)', 'full red capped'],
  [-50, 'rgba(90, 200, 90, 0.8)', 'half-way red'],

  [0, 'rgba(200, 200, 200, 0.8)', 'grey'],
])('.colorFromPercentage(%i)', (a, expected, description) => {
  it(`returns ${expected} ${description})`, () => {
    expect(colorFromPercentage(a, 0.8).toString()).toBe(expected);
  });
});

describe.each([
  // red (diff > 0)
  [30, 60, 'rgba(200, 0, 0, 0.8)'],

  // green (diff < 0%)
  [60, 30, 'rgba(90, 200, 90, 0.8)'],

  // grey (diff == 0)
  [0, 0, 'rgba(200, 200, 200, 0.8)'],
])('.colorBasedOnDiffPercent(%i, %i)', (a, b, expected) => {
  it(`returns ${expected}`, () => {
    expect(colorBasedOnDiffPercent(a, b, 0.8).toString()).toBe(expected);
  });
});
