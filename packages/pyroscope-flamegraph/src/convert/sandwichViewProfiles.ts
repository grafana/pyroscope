/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck

import type { Profile, Flamebearer } from '@pyroscope/models/src';
import { flamebearersToTree } from './flamebearersToTree';

interface TreeNode {
  name: string;
  key: string;
  self: [number];
  total: [number];
  offset?: number;
  children: TreeNode[];
}

interface FlamebearerData {
  maxSelf: number;
  levels: number[][];
  names: string[];
}

export const treeToFlamebearer = (tree: TreeNode): FlamebearerData => {
  const flamebearerData: FlamebearerData = {
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
    return offset || total[0] || 0;
  };

  processNode(tree, 0, 0);

  return flamebearerData;
};

export function calleesFlamebearer(p: Profile, nodeName: string): Flamebearer {
  const tree = flamebearersToTree(p.flamebearer);
  const result: Flamebearer = {
    format: 'single',
    numTicks: 0,
    maxSelf: 100,
    sampleRate: 100,
    names: [],
    levels: [],
    units: p.units,
    spyName: p.spyName,
  };

  const totalNode = {
    name: nodeName,
    key: `/${nodeName}`,
    total: [],
    self: [0],
    children: [],
  };
  const processTree = (node) => {
    if (node.name === nodeName) {
      result.numTicks += node.total[0];

      totalNode.total = [result.numTicks];
      // double check that empty node (total: 0, offset: <value>)
      // should not be passed if node.children.length === 0
      totalNode.children = totalNode.children.concat(node.children);
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
): TreeNode => {
  let result = {};
  let nestedObj = result;
  const emptyLvls = maxLvlNumber - nodesArray.length;

  for (let i = 0; i < emptyLvls; i++) {
    nestedObj.children = [
      { name: '', key: '', total: [0], self: [0], children: [], offset: total },
    ];
    nestedObj = nestedObj.children[0];
  }

  nodesArray.forEach(({ name, ...rest }) => {
    nestedObj.children = [{ name, ...rest, total: [total] }];
    nestedObj = nestedObj.children[0];
  });

  return result.children[0];
};

export function callersFlamebearer(p: Profile, nodeName: string): Flamebearer {
  const tree = flamebearersToTree(p.flamebearer);
  const result: Flamebearer = {
    format: 'single',
    maxSelf: 100,
    sampleRate: 100,
    numTicks: 0,
    names: [],
    levels: [],
    units: p.units,
    spyName: p.spyName,
  };

  const targetFunctionTotals = [];
  const subtrees = [];
  let maxSubtreeLvl = 0;

  const totalNode = {
    name: nodeName,
    key: `/${nodeName}`,
    total: [0],
    self: [0],
    children: [],
  };
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

  subtrees.forEach((v, i) => {
    totalNode.children.push(
      arrayToTree(v, maxSubtreeLvl, targetFunctionTotals[i])
    );
  });

  return { ...result, ...treeToFlamebearer(totalNode) };
}

export function calleesProfile(p: Profile, nodeName: string): Profile {
  const copy = JSON.parse(JSON.stringify(p));
  const calleesResultFlamebearer = calleesFlamebearer(copy, nodeName);

  return {
    version: 1,
    flamebearer: calleesResultFlamebearer,
    metadata: copy.metadata,
    ...calleesResultFlamebearer,
  };
}

export function callersProfile(p: Profile, nodeName: string): Profile {
  const copy = JSON.parse(JSON.stringify(p));

  const callersResultFlamebearer = callersFlamebearer(copy, nodeName);

  return {
    version: 1,
    flamebearer: callersResultFlamebearer,
    metadata: copy.metadata,
    ...callersResultFlamebearer,
  };
}
