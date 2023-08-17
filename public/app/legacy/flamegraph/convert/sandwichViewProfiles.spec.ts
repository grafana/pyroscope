import { treeToFlamebearer, calleesFlamebearer } from './sandwichViewProfiles';
import { flamebearersToTree } from './flamebearersToTree';

import { tree, singleAppearanceTrees } from './testData';
import { Flamebearer } from '@pyroscope/legacy/models';

const flamebearersProps = {
  spyName: 'gospy',
  units: 'samples',
  format: 'single',
  numTicks: 400,
  maxSelf: 150,
  sampleRate: 100,
};

describe('Sandwich view profiles', () => {
  describe('when target function has single tree appearance', () => {
    it('return correct flamebearer with 0 callees', () => {
      const f = treeToFlamebearer(tree);

      const resultCalleesFlamebearer = calleesFlamebearer(
        { ...f, ...flamebearersProps } as Flamebearer,
        'name-5-2'
      );

      const treeToMatchOriginalTree = flamebearersToTree(
        resultCalleesFlamebearer
      );

      expect(treeToMatchOriginalTree).toMatchObject(singleAppearanceTrees.zero);
    });
    it('return correct flamebearer with 1 callee', () => {
      const f = treeToFlamebearer(tree);

      const resultCalleesFlamebearer = calleesFlamebearer(
        { ...f, ...flamebearersProps } as Flamebearer,
        'wwwwwww'
      );

      const treeToMatchOriginalTree = flamebearersToTree(
        resultCalleesFlamebearer
      );

      expect(treeToMatchOriginalTree).toMatchObject(singleAppearanceTrees.one);
    });
    it('return correct flamebearer with multiple callees', () => {
      const f = treeToFlamebearer(tree);

      const resultCalleesFlamebearer = calleesFlamebearer(
        { ...f, ...flamebearersProps } as Flamebearer,
        'name-2-2'
      );

      const treeToMatchOriginalTree = flamebearersToTree(
        resultCalleesFlamebearer
      );

      expect(treeToMatchOriginalTree).toMatchObject(
        singleAppearanceTrees.multiple
      );
    });
  });

  // todo(dogfrogfog): add tests for callers flamegraph when remove top lvl empty node
});
