import type { Profile } from '@pyroscope/models/src';

import { Flamebearer } from '@pyroscope/models/src';
import { flamebearersToTree } from './diffTwoProfiles';
import {
  deltaDiffWrapper,
  deltaDiffWrapperReverse,
} from '../FlameGraph/decode';

function getCalleesFlamebearer(f: Flamebearer, nodeName: string): Flamebearer {
  const result: Flamebearer = {
    format: 'single',
    numTicks: f.numTicks as number,
    maxSelf: 100,
    sampleRate: 100,
    names: [],
    levels: [],
    units: f.units,
    spyName: f.spyName,
  };

  const treeNodeToFlamebearer = (node, level: number, offsetLeft: number) => {
    const { name, children, self, total } = node;
    result.names.push(name);
    result.levels[level] ||= [];
    result.maxSelf = Math.max(result.maxSelf, self[0] || 0);
    result.levels[level] = result.levels[level].concat([
      offsetLeft,
      total[0] || 0,
      self[0] || 0,
      result.names.length - 1,
    ]);

    for (let i = 0; i < children.length; i += 1) {
      const ol = treeNodeToFlamebearer(children[i], level + 1, offsetLeft);
      offsetLeft += ol;
    }
    return total[0];
  };

  const tree = flamebearersToTree(f);

  const findTreeNode = (node) => {
    const { name, children, total, self } = node;
    if (!result.levels.length && name === nodeName) {
      treeNodeToFlamebearer(node, 0, 0);
      result.numTicks = total[0];
      result.maxSelf = self[0];
    }
    for (let i = 0; i < children.length; i += 1) {
      findTreeNode(children[i]);
    }
  };

  findTreeNode(tree);

  return result;
}

// should return both callees and callers profiles after implementation
export function sandwichViewProfiles(
  p: Profile | any,
  nodeName: string
): Profile {
  const copy = JSON.parse(JSON.stringify(p));
  copy.flamebearer.levels = deltaDiffWrapper('single', copy.flamebearer.levels);
  const calleesFlamebearer = getCalleesFlamebearer(copy.flamebearer, nodeName);
  calleesFlamebearer.levels = deltaDiffWrapperReverse(
    'single',
    calleesFlamebearer.levels
  );

  return {
    version: 1,
    flamebearer: calleesFlamebearer,
    metadata: copy.metadata,
  };
}
