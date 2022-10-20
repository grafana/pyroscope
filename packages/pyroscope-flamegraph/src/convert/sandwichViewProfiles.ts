/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck

// todo(dogfrogfog):
// 1. add types
// 2. remove "total" node duplication
// 3. refactor arrayToTree function

import type { Profile, Flamebearer } from '@pyroscope/models/src';
import {
  deltaDiffWrapper,
  deltaDiffWrapperReverse,
} from '../FlameGraph/decode';
import { flamebearersToTree } from './flamebearersToTree';
import { tree as tree1 } from './testData';

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
  const tree = flamebearersToTree(f);
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

  // totalNode - node to accumulate values
  const totalNode = {
    children: [],
    total: [],
    name: 'total',
    key: '/total',
    self: [0],
  };
  // use recursion to treverse the tree and fill totalNode.children with selected function nodes
  const processTree = (node) => {
    if (node.name === nodeName) {
      result.numTicks += node.total[0];

      totalNode.total = [result.numTicks];
      totalNode.children.push(node);
    }
    for (let i = 0; i < node.children.length; i += 1) {
      processTree(node.children[i]);
    }
  };
  processTree(tree1);

  return { ...result, ...treeToFlamebearer(totalNode) };
}

// todo(dogfrogfog): add types
const arrayToTree = (nodesArray) => {
  let result = {};
  let nestedObj = result;
  nodesArray.forEach(({ name, ...rest }) => {
    nestedObj.children = [{ name, ...rest }];
    nestedObj = nestedObj.children[0];
  });

  return result.children[0];
};

export function callersFlamebearer(
  f: Flamebearer,
  nodeName: string
): Flamebearer {
  const tree = flamebearersToTree(f);
  const result: Flamebearer = {
    format: 'single',
    maxSelf: 100,
    sampleRate: 100,
    numTicks: 0,
    names: [],
    levels: [],
    units: f.units,
    spyName: f.spyName,
  };

  let targetFunctionTotalSelf = 0;
  const totalNode = { total: [0], self: [0], name: '', children: [] };
  const processTree = (node, parentNodes = []) => {
    const { name, children, total, ...rest } = node;
    const currentNode = {
      children: [],
      name,
      total,
      ...rest,
    };

    if (name === nodeName) {
      targetFunctionTotalSelf += self[0];
      const subTree = arrayToTree([...parentNodes, currentNode]);

      result.numTicks += subTree.total[0];
      totalNode.children.push(subTree);
    }

    for (let i = 0; i < children.length; i += 1) {
      processTree(children[i], [...parentNodes, currentNode]);
    }
  };
  processTree(tree1);

  return { ...result, ...treeToFlamebearer(totalNode) };
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

export function callersProfile(p: Profile, nodeName: string): Profile {
  const copy = JSON.parse(JSON.stringify(p));

  copy.flamebearer.levels = deltaDiffWrapper('single', copy.flamebearer.levels);
  const callersResultFlamebearer = callersFlamebearer(
    copy.flamebearer,
    nodeName
  );
  callersResultFlamebearer.levels = deltaDiffWrapperReverse(
    'single',
    callersResultFlamebearer.levels
  );

  return {
    version: 1,
    flamebearer: callersResultFlamebearer,
    metadata: copy.metadata,
  };
}
