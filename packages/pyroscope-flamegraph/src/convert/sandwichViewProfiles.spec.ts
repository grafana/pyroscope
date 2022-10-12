import { calleesFlamebearer } from './sandwichViewProfiles';
import { flamebearersToTree } from './convert';

import { tree1 } from './testData';

jest.mock('./convert', () => ({
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
        [0, 400, 100, 1],
        [0, 300, 0, 2],
        [0, 150, 150, 3, 150, 150, 150, 4],
      ],
      maxSelf: 150,
      names: [
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

  it('should return correct callees flamebearer (multiple target function name appearancies)', () => {
    expect(
      calleesFlamebearer({} as any, 'specific-function-name')
    ).toMatchObject({
      format: 'single',
      levels: [
        [0, 600, 0, 0, 600, 200, 200, 1, 800, 300, 0, 2],
        [0, 200, 200, 3, 200, 400, 400, 4, 800, 150, 150, 5, 950, 150, 150, 6],
      ],
      maxSelf: 200,
      names: [
        'specific-function-name',
        'specific-function-name',
        'name-3-2',
        'specific-function-name',
        'name-5-1',
        'name-5-2',
      ],
      numTicks: 1100,
      sampleRate: 100,
    });
  });
});
