import { calleesFlamebearer } from './sandwichViewProfiles';
import { flamebearersToTree } from './flamebearersToTree';

import { tree1 } from './testData';

jest.mock('./flamebearersToTree', () => ({
  flamebearersToTree: jest.fn(),
}));

describe('Sandwich view profiles', () => {
  beforeAll(() => {
    (flamebearersToTree as jest.Mock).mockReturnValue(tree1);
  });

  it('should return correct callees flamebearer (single target function name appearance)', () => {
    expect(calleesFlamebearer({} as any, 'name-2-2')).toMatchObject({
      format: 'single',
      levels: [
        [0, 400, 0, 0],
        [0, 400, 0, 1],
        [0, 400, 100, 2],
        [0, 300, 0, 3],
        [0, 150, 150, 4, 150, 150, 150, 5],
      ],
      maxSelf: 150,
      names: [
        'total',
        'name-2-2',
        'name-3-1',
        'specific-function-name',
        'name-5-1',
        'name-5-2',
      ],
      numTicks: 400,
      sampleRate: 100,
    });
  });

  it('should return correct callees flamebearer (multiple target function name appearances)', () => {
    expect(
      calleesFlamebearer({} as any, 'specific-function-name')
    ).toMatchObject({
      format: 'single',
      levels: [
        // 1st level is total to accumulate nodes with same name
        [0, 1100, 0, 0],
        [0, 600, 0, 1, 600, 200, 200, 4, 800, 300, 0, 5],
        [0, 200, 200, 2, 200, 400, 400, 3, 800, 150, 150, 6, 950, 150, 150, 7],
      ],
      maxSelf: 400,
      names: [
        'total',
        'specific-function-name',
        'specific-function-name',
        'name-3-2',
        'specific-function-name',
        'specific-function-name',
        'name-5-1',
        'name-5-2',
      ],
      numTicks: 1100,
      sampleRate: 100,
    });
  });
});
