/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck

import type { Profile, Flamebearer } from '@pyroscope/models/src';
import {
  deltaDiffWrapper,
  deltaDiffWrapperReverse,
} from '../FlameGraph/decode';
import { flamebearersToTree } from './flamebearersToTree';

export const treeToFlamebearer = (tree) => {
  const flamebearerData: {
    maxSelf: number;
    levels: number[][];
    names: string[];
  } = {
    maxSelf: 100,
    names: [],
    levels: [],
  };

  const processNode = (node: any, level: number, offsetLeft: number) => {
    const { name, children, self, total } = node;
    flamebearerData.names.push(name);
    flamebearerData.levels[level] ||= [];
    flamebearerData.maxSelf = Math.max(flamebearerData.maxSelf, self[0] || 0);
    flamebearerData.levels[level] = flamebearerData.levels[level].concat([
      offsetLeft,
      total[0] || 0,
      self[0] || 0,
      flamebearerData.names.length - 1,
    ]);

    for (let i = 0; i < children.length; i += 1) {
      const ol = processNode(children[i], level + 1, offsetLeft);
      offsetLeft += ol;
    }
    return total[0] || 0;
  };

  processNode(tree, 0, 0);

  return flamebearerData;
};

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

  const combinedNode = nodesArray.reduce(
    (acc: any, node: any) => {
      // to prevent displaying 2 total lines
      if (node.name !== 'total') {
        acc.children.push(node);
        acc.total[0] += node.total[0];
      } else {
        return node;
      }

      return acc;
    },
    { total: [0], self: [0], key: '/total', name: 'total', children: [] }
  );

  const flamebearersData = treeToFlamebearer(combinedNode);

  return { ...result, ...flamebearersData };
}

export function calleesProfile(p: Profile, nodeName: string): Profile {
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

  return {
    version: 1,
    flamebearer: calleesResultFlamebearer,
    metadata: copy.metadata,
  };
}
