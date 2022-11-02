/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck

import type { Flamebearer } from '@pyroscope/models/src';
import { flamebearersToTree } from './flamebearersToTree';

export interface TreeNode {
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

  const processNode = (node: TreeNode, level: number, offsetLeft: number) => {
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

const arrayToTree = (nodesArray: TreeNode[], total: number): TreeNode => {
  const result = {};
  let nestedObj = result;

  nodesArray.forEach(({ name, ...rest }) => {
    const nextNode = { name, ...rest, total: [total] };
    nestedObj.children = [nextNode];
    nestedObj = nextNode;
  });

  return result.children[0];
};

function dedupTree(node) {
  const childrenMap = {};
  for (let i = 0; i < node.children.length; i += 1) {
    childrenMap[node.children[i].name] ||= node.children[i];
  }
  for (let i = 0; i < node.children.length; i += 1) {
    const currentNode = node.children[i];
    const existingNode = childrenMap[node.children[i].name];
    if (existingNode !== currentNode) {
      existingNode.total[0] += currentNode.total[0];
      existingNode.self[0] += currentNode.self[0];
      existingNode.children = existingNode.children.concat(
        currentNode.children
      );
    }
  }
  node.children = Object.values(childrenMap);
  for (let i = 0; i < node.children.length; i += 1) {
    dedupTree(node.children[i]);
  }
}

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
      totalNode.children = totalNode.children.concat(node.children);
    }
    for (let i = 0; i < node.children.length; i += 1) {
      processTree(node.children[i]);
    }
  };
  processTree(tree);
  dedupTree(totalNode);

  return { ...result, ...treeToFlamebearer(totalNode) };
}

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
    }

    for (let i = 0; i < node.children.length; i += 1) {
      processTree(node.children[i], currentSubtree);
    }
  };
  processTree(tree);

  subtrees.forEach((v, i) => {
    totalNode.children.push(arrayToTree(v.reverse(), targetFunctionTotals[i]));
  });

  dedupTree(totalNode);

  const flamebearer = treeToFlamebearer(totalNode);
  flamebearer.levels = flamebearer.levels.reverse().slice(0, -1);

  return { ...result, ...flamebearer };
}

export function calleesProfile(f: Flamebearer, nodeName: string): Flamebearer {
  const copy = JSON.parse(JSON.stringify(f));
  const calleesResultFlamebearer = calleesFlamebearer(copy, nodeName);

  return {
    version: 1,
    // flamebearer: calleesResultFlamebearer,
    // metadata: copy.metadata,
    ...calleesResultFlamebearer,
  };
}

export function callersProfile(f: Flamebearer, nodeName: string): Flamebearer {
  const copy = JSON.parse(JSON.stringify(f));

  const callersResultFlamebearer = callersFlamebearer(copy, nodeName);

  return {
    version: 1,
    // flamebearer: callersResultFlamebearer,
    // metadata: copy.metadata,
    ...callersResultFlamebearer,
  };
}
