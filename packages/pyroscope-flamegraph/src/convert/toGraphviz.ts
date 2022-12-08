import { Profile } from 'packages/pyroscope-models/src';

import decodeFlamebearer from '../FlameGraph/decode';
import { flamebearersToTree, TreeNode } from './flamebearersToTree';

function renderLabels(obj) {
  const labels: string[] = [];
  for (const key in obj) {
    labels.push(`${key}="${obj[key]}"`);
  }
  return `[${labels.join(' ')}]`;
}

const baseFontSize = 8;
const maxFontGrowth = 16;

function renderNode(n: TreeNode, index: number, maxSelf: number): string {
  const self = n.self[0];
  const total = n.total[0];

  const name = n.name.replace(/"/g, '\\"');
  const dur = '160ms'; // TODO
  const percent = '34.04%'; // TODO
  const fontsize = 10; //baseFontSize + int(Math.ceil(maxFontGrowth * Math.sqrt(float64(abs64(self))/maxSelf))); // TODO
  const color = '#b23100'; // TODO
  const fillcolor = '#eddbd5'; // TODO

  const labels = {
    label: `${name} \n ${dur} (${percent})`,
    id: `node${index}`,
    shape: 'box',
    tooltip: `${name} (${dur})`,
  };
  return `N${index} ${renderLabels(labels)}`;
}

function renderEdge(
  src: TreeNode,
  dst: TreeNode,
  srcIndex: number,
  dstIndex: number,
  total: number
): string {
  const srcName = src.name.replace(/"/g, '\\"');
  const dstName = dst.name.replace(/"/g, '\\"');
  const dur = '10ms'; // TODO
  const edgeWeight = 3; // TODO
  const weight = (edgeWeight * 100) / total;
  const penwidth = (edgeWeight * 5) / total;
  const color = '#b2ac9f'; // TODO
  const tooltip = `${srcName} -> ${dstName} (${dur})`;

  const labels = {
    label: dur,
    weight: weight,
    penwidth: penwidth,
    tooltip: tooltip,
    labeltooltip: tooltip,
  };
  return `N${srcIndex} -> N${dstIndex} ${renderLabels(labels)}`;
}

export default function toGraphviz(p: Profile): string {
  p = decodeFlamebearer(p);
  const tree = flamebearersToTree(p.flamebearer);

  const nodes: string[] = [];
  const edges: string[] = [];

  const maxSelf = 100; // TODO

  function processNode(n: TreeNode): number {
    const srcIndex = nodes.length;
    nodes.push(renderNode(n, srcIndex, maxSelf));
    for (const child of n.children) {
      const dstIndex = processNode(child);
      const total = 100; // TODO
      edges.push(renderEdge(n, child, srcIndex, dstIndex, total));
    }
    return srcIndex;
  }

  processNode(tree);

  return `digraph "unnamed" {
    ${nodes.join('\n')}
    ${edges.join('\n')}
  }`;
}
