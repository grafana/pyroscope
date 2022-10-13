import type { Profile, Flamebearer } from '@pyroscope/models/src';
import {
  deltaDiffWrapper,
  deltaDiffWrapperReverse,
} from '../FlameGraph/decode';
import { flamebearersToTree } from './flamebearersToTree';

export function calleesFlamebearer(
  f: Flamebearer,
  nodeName: string
): Flamebearer {
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
  let targetTreeNode: any;
  const sameNameNodesArr: any = [];

  const tree = flamebearersToTree(f);

  const processTree = (node: any) => {
    const { name, children, total, self } = node;
    if (name === nodeName) {
      if (!targetTreeNode) {
        targetTreeNode = node;
        result.numTicks = total[0];
        result.maxSelf = self[0];
      }
      sameNameNodesArr.push(node);
    }
    for (let i = 0; i < children.length; i += 1) {
      processTree(children[i]);
    }
  };

  processTree(tree);

  const processNode = (node: any, level: number, offsetLeft: number) => {
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
      const ol = processNode(children[i], level + 1, offsetLeft);
      offsetLeft += ol;
    }
    return total[0] || 0;
  };

  processNode(targetTreeNode, 0, 0);

  return result;
}

export function sandwichViewProfiles(
  p: Profile,
  nodeName: string
): [Profile, Profile] {
  const copy = JSON.parse(JSON.stringify(p));
  copy.flamebearer.levels = deltaDiffWrapper('single', copy.flamebearer.levels);
  const calleesResultFlamebearer = calleesFlamebearer(
    copy.flamebearer,
    nodeName
  );
  calleesResultFlamebearer.levels = deltaDiffWrapperReverse(
    'single',
    calleesResultFlamebearer.levels
  );

  return [
    {
      version: 1,
      flamebearer: calleesResultFlamebearer,
      metadata: copy.metadata,
    },
    {
      // not implemented
    } as Profile,
  ];
}
