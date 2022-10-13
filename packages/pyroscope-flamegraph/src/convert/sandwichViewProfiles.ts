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
    numTicks: 0,
    maxSelf: 100,
    sampleRate: 100,
    names: [],
    levels: [],
    units: f.units,
    spyName: f.spyName,
  };
  const nodesArray: any = [];

  const tree = flamebearersToTree(f);

  const processTree = (node: any) => {
    const { name, children, total } = node;
    if (name === nodeName) {
      nodesArray.push(node);
      result.numTicks += total[0];
    }
    for (let i = 0; i < children.length; i += 1) {
      processTree(children[i]);
    }
  };

  processTree(tree);

  const combinedNode =
    nodesArray.length > 1
      ? nodesArray.reduce(
          (acc: any, node: any) => {
            acc.children.push(node);
            acc.total[0] += node.total[0];

            return acc;
          },
          { total: [0], self: [0], key: '/total', name: 'total', children: [] }
        )
      : nodesArray[0];

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

  processNode(combinedNode, 0, 0);

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
