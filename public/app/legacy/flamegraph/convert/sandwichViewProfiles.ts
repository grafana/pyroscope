import type { Flamebearer } from '@pyroscope/legacy/models';
import { flamebearersToTree, TreeNode } from './flamebearersToTree';

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
  const result = {} as TreeNode;
  let nestedObj = result;

  nodesArray.forEach(({ name, ...rest }) => {
    const nextNode = { name, ...rest, total: [total] };
    nestedObj.children = [nextNode];
    nestedObj = nextNode;
  });

  return result.children[0];
};

function dedupTree(node: TreeNode) {
  const childrenMap = new Map<string, TreeNode>();
  for (let i = 0; i < node.children.length; i += 1) {
    if (!childrenMap.has(node.children[i].name)) {
      childrenMap.set(node.children[i].name, node.children[i]);
    }
  }

  for (let i = 0; i < node.children.length; i += 1) {
    const currentNode = node.children[i];
    const existingNode = childrenMap.get(node.children[i].name);
    if (existingNode && existingNode !== currentNode) {
      existingNode.total[0] += currentNode.total[0];
      existingNode.self[0] += currentNode.self[0];
      existingNode.children = existingNode.children.concat(
        currentNode.children
      );
    }
  }
  node.children = Array.from(childrenMap.values());
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
  } as TreeNode;
  const processTree = (node: TreeNode) => {
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

  const targetFunctionTotals: number[] = [];
  const subtrees: TreeNode[][] = [];

  const totalNode = {
    name: nodeName,
    key: `/${nodeName}`,
    total: [0],
    self: [0],
    children: [],
  } as TreeNode;
  const processTree = (node: TreeNode, parentNodes: TreeNode[] = []) => {
    const currentSubtree = parentNodes.concat([
      { ...node, children: [] } as TreeNode,
    ]);

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

  // 1. we first make a regular tree
  subtrees.forEach((v, i) => {
    totalNode.children.push(arrayToTree(v.reverse(), targetFunctionTotals[i]));
  });

  // 2. that allows us to use the same dedup function
  dedupTree(totalNode);

  const flamebearer = treeToFlamebearer(totalNode);

  // 3. then we reverse levels so that tree goes from bottom to top
  flamebearer.levels = flamebearer.levels.reverse().slice(0, -1);

  return { ...result, ...flamebearer };
}

export function calleesProfile(f: Flamebearer, nodeName: string): Flamebearer {
  const copy = JSON.parse(JSON.stringify(f));
  const calleesResultFlamebearer = calleesFlamebearer(copy, nodeName);

  return calleesResultFlamebearer;
}

export function callersProfile(f: Flamebearer, nodeName: string): Flamebearer {
  const copy = JSON.parse(JSON.stringify(f));

  const callersResultFlamebearer = callersFlamebearer(copy, nodeName);

  return callersResultFlamebearer;
}
