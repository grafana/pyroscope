import { treeToFlamebearer, calleesFlamebearer } from './sandwichViewProfiles';
import { flamebearersToTree } from './flamebearersToTree';
import type { Flamebearer } from '@pyroscope/models/src';

import { tree, name22FunctionTreeWithTotal } from './testData';

describe('Sandwich view profiles', () => {
  it('should return correct callees flamebearer for single function appearance', () => {
    const f = treeToFlamebearer(tree);

    const resultCalleesFlamebearer = calleesFlamebearer(
      {
        ...f,
        spyName: 'gospy',
        units: 'samples',
        format: 'single',
      } as Flamebearer,
      'name-2-2'
    );

    const treeToMatchOriginalTree = flamebearersToTree(
      resultCalleesFlamebearer
    );

    expect(treeToMatchOriginalTree).toMatchObject(name22FunctionTreeWithTotal);
  });
});
