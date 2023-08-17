import type { Profile } from '@pyroscope/legacy/models';
import {
  decodeFlamebearer,
  deltaDiffWrapperReverse,
} from '../FlameGraph/decode';
import { flamebearersToTree, TreeNode } from './flamebearersToTree';

function subtractFlamebearer(
  f1: Profile['flamebearer'],
  f2: Profile['flamebearer']
): Profile['flamebearer'] {
  const result: Profile['flamebearer'] = {
    numTicks: 0,
    maxSelf: 0,
    names: [],
    levels: [],
  };

  const tree = flamebearersToTree(f1, f2);

  const updateNumbers = (node: TreeNode): number => {
    // self is easy
    node.self[0] = Math.max((node.self[0] || 0) - (node.self[1] || 0), 0);
    result.numTicks += node.self[0];

    // total needs to be recalculated using children
    if (node.children.length === 0) {
      node.total[0] = Math.max((node.total[0] || 0) - (node.total[1] || 0), 0);
    } else {
      let total = node.self[0];
      for (let i = 0; i < node.children.length; i += 1) {
        total += updateNumbers(node.children[i]);
      }
      node.total[0] = total;
    }

    return node.total[0];
  };

  updateNumbers(tree);

  const processNode = (node: TreeNode, level: number, offset: number) => {
    const { name, children, self, total } = node;
    result.levels[level] ||= [];
    const newSelf = self[0];
    const newTotal = total[0];
    result.maxSelf = Math.max(result.maxSelf, newSelf);
    if (newTotal === 0) {
      return 0;
    }
    result.names.push(name);
    result.levels[level] = result.levels[level].concat([
      offset,
      newTotal,
      newSelf,
      result.names.length - 1,
    ]);
    for (let i = 0; i < children.length; i += 1) {
      const offsetAddition = processNode(children[i], level + 1, offset);
      offset += offsetAddition;
    }
    return newTotal;
  };

  processNode(tree, 0, 0);
  return result;
}

// this functions expects two compressed (before delta diff) profiles,
//   and returns a compressed profile
export function subtract(p1: Profile, p2: Profile): Profile {
  p1 = decodeFlamebearer(p1);
  p2 = decodeFlamebearer(p2);

  const resultFlamebearer = subtractFlamebearer(p1.flamebearer, p2.flamebearer);
  resultFlamebearer.levels = deltaDiffWrapperReverse(
    'single',
    resultFlamebearer.levels
  );
  const metadata = { ...p1.metadata };
  metadata.format = 'single';
  return {
    version: 1,
    flamebearer: resultFlamebearer,
    metadata,
  };
}
