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

interface TreeNode<T> {
  name: string;
  key: string;
  self: [number];
  total: [number];
  offset?: number;
  children: T[];
}

export const treeToFlamebearer = (tree: TreeNode<TreeNode>): Flamebearer => {
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
    const { name, children, self, total, offset } = node;
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
    return total[0] || offset || 0;
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
  processTree(tree);

  return { ...result, ...treeToFlamebearer(totalNode) };
}

const arrayToTree = (
  nodesArray: TreeNode[],
  maxLvlNumber: number,
  total: number
): TreeNode<TreeNode> => {
  let result = {};
  let nestedObj = result;
  const emptyLvls = maxLvlNumber - nodesArray.length;

  for (let i = 0; i < emptyLvls; i++) {
    // todo: fix undefined
    nestedObj.children = [
      { total: [undefined], self: [0], name: '', children: [], offset: total },
    ];
    nestedObj = nestedObj.children[0];
  }

  nodesArray.forEach(({ name, ...rest }) => {
    // check time(%) values
    nestedObj.children = [{ name, ...rest, total: [total] }];
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

  const targetFunctionTotals = [];
  const subtrees = [];
  let maxSubtreeLvl = 0;

  const processTree = (node, parentNodes = []) => {
    const currentSubtree = parentNodes.concat([{ ...node, children: [] }]);

    if (node.name === nodeName) {
      subtrees.push(currentSubtree);
      targetFunctionTotals.push(node.total[0]);
      result.numTicks += node.total[0];

      if (maxSubtreeLvl < currentSubtree.length) {
        maxSubtreeLvl = currentSubtree.length;
      }
    }

    for (let i = 0; i < node.children.length; i += 1) {
      processTree(node.children[i], currentSubtree);
    }
  };
  processTree(tree);

  // todo(dogfrogfog): top lvl accumulator should be removed
  const totalNode = { total: 0, self: [0], name: '', children: [] };
  subtrees.forEach((v, i) => {
    totalNode.children.push(
      arrayToTree(v, maxSubtreeLvl, targetFunctionTotals[i])
    );
  });

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
