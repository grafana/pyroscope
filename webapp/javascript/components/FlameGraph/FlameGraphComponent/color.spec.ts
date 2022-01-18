import Color from 'color';
import {
  colorBasedOnDiffPercent,
  colorFromPercentage,
  NewDiffColor,
} from './color';
import { DefaultPalette } from './colorPalette';

describe.each([
  // red (diff > 0)
  [30, 60, DefaultPalette.badColor.toString()],

  // green (diff < 0%)
  [60, 0, DefaultPalette.goodColor.toString()],

  // grey (diff == 0)
  [0, 0, DefaultPalette.neutralColor.toString()],
])('.colorBasedOnDiffPercent(%i, %i)', (a, b, expected) => {
  it(`returns ${expected}`, () => {
    expect(colorBasedOnDiffPercent(DefaultPalette, a, b).rgb().toString()).toBe(
      expected
    );
  });
});

describe('NewDiffColor with white-to-black example palette', () => {
  describe.each([
    [-100, 'rgb(255, 255, 255)'],
    [0, 'rgb(128, 128, 128)'],
    [100, 'rgb(0, 0, 0)'],
  ])('.NewDiffColor(%i)', (a, expected) => {
    it(`returns ${expected}`, () => {
      const color = NewDiffColor({
        goodColor: Color('white'),
        neutralColor: Color('grey'),
        badColor: Color('black'),
      });

      expect(color(a).rgb().toString()).toBe(expected);
    });
  });
});
