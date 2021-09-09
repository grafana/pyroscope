import {colorBasedOnDiff} from '../javascript/components/FlameGraph/FlameGraphComponent/color';

describe.each([
  [ -300, 100, 'rgba(0, 200, 0, 0.8)' ],
  [ -200, 100, 'rgba(0, 200, 0, 0.8)' ],
  [ -100, 100, 'rgba(0, 200, 0, 0.8)'     ],
  [ -50,  100, 'rgba(59, 200, 59, 0.8)'   ],
  [ 0,    100, 'rgba(200, 200, 200, 0.8)' ],
  [ 50,   100, 'rgba(200, 59, 59, 0.8)'   ],
  [ 100,  100, 'rgba(200, 0, 0, 0.8)'     ],
  [ 200,  100, 'rgba(200, 0, 0, 0.8)' ],
  [ 300,  100, 'rgba(200, 0, 0, 0.8)' ],
])('.colorBasedOnDiff(%i, %i)', (a, b, expected) => {
  it(`returns ${expected}`, () => {
    expect(colorBasedOnDiff(a,b,0.8).toString()).toBe(expected);
  });
});
