import { treeToFlamebearer, calleesFlamebearer } from './sandwichViewProfiles';
import { flamebearersToTree } from './flamebearersToTree';

import { tree, name22FunctionTreeWithTotal } from './testData';

describe('Sandwich view profiles', () => {
  it('should return correct callees flamebearer for single function appearance', () => {
    const f = treeToFlamebearer(tree);

    const resultCalleesFlamebearer = calleesFlamebearer(
      // @ts-ignore
      {
        ...f,
        spyName: 'gospy',
        units: 'samples',
        format: 'single',
      },
      'name-2-2'
    );

    const treeToMatchOriginalTree = flamebearersToTree(
      resultCalleesFlamebearer
    );

    expect(treeToMatchOriginalTree).toMatchObject(name22FunctionTreeWithTotal);
  });
});
