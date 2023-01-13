import { Profile } from 'packages/pyroscope-models/src';

import { flamebearersToTree, TreeNode } from './flamebearersToTree';
import { getFormatter } from '../format/format';

const nodeFraction = 0.005;
const edgeFraction = 0.001;
const maxNodes = 80;

// have to specify font name here, otherwise renderer won't size boxes properly
// const fontName = "SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace";
const fontName = '';

function renderLabels(obj) {
  const labels: string[] = [];
  for (const key in obj) {
    labels.push(`${key}="${escapeForDot(String(obj[key] || ''))}"`);
  }
  return `[${labels.join(' ')}]`;
}

const baseFontSize = 8;
const maxFontGrowth = 16;

function formatPercent(a: number, b: number): string {
  return ((a * 100) / b).toFixed(2) + '%';
}

type sampleFormatter = (dur: number) => string;

// dotColor returns a color for the given score (between -1.0 and
// 1.0), with -1.0 colored green, 0.0 colored grey, and 1.0 colored
// red. If isBackground is true, then a light (low-saturation)
// color is returned (suitable for use as a background color);
// otherwise, a darker color is returned (suitable for use as a
// foreground color).
function dotColor(score: number, isBackground: boolean): string {
  // A float between 0.0 and 1.0, indicating the extent to which
  // colors should be shifted away from grey (to make positive and
  // negative values easier to distinguish, and to make more use of
  // the color range.)
  const shift = 0.7;

  // Saturation and value (in hsv colorspace) for background colors.
  const bgSaturation = 0.1;
  const bgValue = 0.93;

  // Saturation and value (in hsv colorspace) for foreground colors.
  const fgSaturation = 1.0;
  const fgValue = 0.7;

  // Choose saturation and value based on isBackground.
  let saturation: number;
  let value: number;
  if (isBackground) {
    saturation = bgSaturation;
    value = bgValue;
  } else {
    saturation = fgSaturation;
    value = fgValue;
  }

  // Limit the score values to the range [-1.0, 1.0].
  score = Math.max(-1.0, Math.min(1.0, score));

  // Reduce saturation near score=0 (so it is colored grey, rather than yellow).
  if (Math.abs(score) < 0.2) {
    saturation *= Math.abs(score) / 0.2;
  }

  // Apply 'shift' to move scores away from 0.0 (grey).
  if (score > 0.0) {
    score = Math.pow(score, 1.0 - shift);
  }
  if (score < 0.0) {
    score = -Math.pow(-score, 1.0 - shift);
  }

  let r: number;
  let g: number;
  let b: number; // red, green, blue
  if (score < 0.0) {
    g = value;
    r = value * (1 + saturation * score);
  } else {
    r = value;
    g = value * (1 - saturation * score);
  }
  b = value * (1 - saturation);
  return `#${Math.floor(r * 255.0)
    .toString(16)
    .padStart(2, '0')}${Math.floor(g * 255.0)
    .toString(16)
    .padStart(2, '0')}${Math.floor(b * 255.0)
    .toString(16)
    .padStart(2, '0')}`;
}

function renderNode(
  format: sampleFormatter,
  n: GraphNode,
  maxSelf: number,
  maxTotal: number
): string {
  const self = n.self;
  const total = n.total;

  const name = n.name.replace(/"/g, '\\"');
  const dur = format(self);
  const fontsize =
    baseFontSize + Math.ceil(maxFontGrowth * Math.sqrt(self / maxSelf));
  const color = dotColor(total / maxTotal, false);
  const fillcolor = dotColor(total / maxTotal, true);

  const label = formatNodeLabel(format, name, self, total, maxTotal);

  // console.log(color, fillcolor);

  const labels = {
    label: label,
    id: `node${n.index}`,
    fontsize: fontsize,
    shape: 'box',
    tooltip: `${name} (${dur})`,
    color: color,
    fontname: fontName,
    fillcolor: fillcolor,
  };
  return `N${n.index} ${renderLabels(labels)}`;
}

function escapeForDot(str: string) {
  return str.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

function pathBasename(p) {
  return p.replace(/.*\//, '');
}

function formatNodeLabel(format: sampleFormatter, name, self, total, maxTotal) {
  var label: string = '';
  label = pathBasename(name) + '\n';

  var selfValue = format(self);
  if (self != 0) {
    label = label + selfValue + ' (' + formatPercent(self, maxTotal) + ')';
  } else {
    label = label + '0';
  }
  var totalValue = selfValue;
  if (total != self) {
    if (self != 0) {
      label = label + '\n';
    } else {
      label = label + ' ';
    }
    totalValue = format(total);
    label =
      label + 'of ' + totalValue + ' (' + formatPercent(total, maxTotal) + ')';
  }

  return label;
}

function renderEdge(
  sampleFormatter: sampleFormatter,
  edge: GraphEdge,
  maxTotal: number
): string {
  const srcName = edge.from.name.replace(/"/g, '\\"');
  const dstName = edge.to.name.replace(/"/g, '\\"');
  const edgeWeight = edge.weight; // TODO
  const dur = sampleFormatter(edge.weight); // TODO
  const weight = 1 + (edgeWeight * 100) / maxTotal;
  const penwidth = 1 + (edgeWeight * 5) / maxTotal;
  const color = dotColor(edgeWeight / maxTotal, false);
  const tooltip = `${srcName} -> ${dstName} (${dur})`;

  const labels = {
    label: dur,
    weight: weight,
    penwidth: penwidth,
    tooltip: tooltip,
    labeltooltip: tooltip,
    fontname: fontName,
    color: color,
    style: edge.residual ? 'dotted' : '',
  };
  return `N${edge.from.index} -> N${edge.to.index} ${renderLabels(labels)}`;
}

type GraphNode = {
  name: string;
  index: number;
  self: number;
  total: number;
  parents: GraphEdge[];
  children: GraphEdge[];
};

type GraphEdge = {
  from: GraphNode;
  to: GraphNode;
  weight: number;
  residual: boolean;
};

export default function toGraphviz(p: Profile): string {
  const tree = flamebearersToTree(p.flamebearer);

  const nodes: string[] = [];
  const edges: string[] = [];

  function calcMaxAndSumValues(
    n: TreeNode,
    maxSelf: number,
    maxTotal: number,
    sumSelf: number,
    sumTotal: number
  ): [number, number, number, number] {
    for (const child of n.children) {
      const [newMaxSelf, newMaxTotal] = calcMaxAndSumValues(
        child,
        maxSelf,
        maxTotal,
        sumSelf,
        sumTotal
      );
      maxSelf = Math.max(maxSelf, newMaxSelf);
      maxTotal = Math.max(maxTotal, newMaxTotal);
    }

    maxSelf = Math.max(maxSelf, n.self[0]);
    maxTotal = Math.max(maxTotal, n.total[0]);
    sumSelf += n.self[0];
    sumTotal += n.total[0];

    return [maxSelf, maxTotal, sumSelf, sumTotal];
  }

  const [maxSelf, maxTotal, sumSelf, sumTotal] = calcMaxAndSumValues(
    tree,
    0,
    0,
    0,
    0
  );
  const { sampleRate, units } = p.metadata;
  const formatter = getFormatter(maxTotal, sampleRate, units);

  const formatFunc = (dur: number): string => {
    return formatter.format(dur, sampleRate, true);
  };

  // we first turn tree into a graph
  let graphNodes: { [key: string]: GraphNode } = {};
  let graphEdges: { [key: string]: GraphEdge } = {};
  let nodesTotal = 0;
  function treeToGraph(n: TreeNode, seenNodes: string[]): GraphNode {
    if (seenNodes.indexOf(n.name) === -1) {
      if (!graphNodes[n.name]) {
        graphNodes[n.name] = {
          index: nodesTotal++,
          name: n.name,
          self: n.self[0],
          total: n.total[0],
          parents: [],
          children: [],
        };
      } else {
        graphNodes[n.name].self += n.self[0];
        graphNodes[n.name].total += n.total[0];
      }
    }

    for (const child of n.children) {
      const childNode = treeToGraph(child, seenNodes.concat([n.name]));
      const childKey = n.name + '/' + child.name;
      if (child.name !== n.name) {
        if (!graphEdges[childKey]) {
          graphEdges[childKey] = {
            from: graphNodes[n.name],
            to: childNode,
            weight: child.total[0],
            residual: false,
          };
        } else {
          graphEdges[childKey].weight += child.total[0];
        }
        childNode.parents.push(graphEdges[childKey]);
        graphNodes[n.name].children.push(graphEdges[childKey]);
      }
    }
    return graphNodes[n.name];
  }

  // skip "total" node
  for (const child of tree.children) {
    treeToGraph(child, []);
  }

  // next is we need to trim graph to remove small nodes
  const nodeCutoff = sumTotal * nodeFraction;
  const edgeCutoff = sumTotal * edgeFraction;

  console.log('nodeCutoff ', nodeCutoff);
  console.log('edgeCutoff ', edgeCutoff);

  for (const key in graphNodes) {
    if (graphNodes[key].total < nodeCutoff) {
      delete graphNodes[key];
    }
  }

  // next is we limit total number of nodes

  function entropyScore(n: GraphNode): number {
    let score = 0;

    if (n.parents.length == 0) {
      score += 1;
    } else {
      score += edgeEntropyScore(n.parents, 0);
    }

    if (n.children.length == 0) {
      score += 1;
    } else {
      score += edgeEntropyScore(n.children, n.self);
    }

    return score * n.total + n.self;
  }
  function edgeEntropyScore(edges: GraphEdge[], self: number) {
    let score = 0;
    let total = self;
    for (const e of edges) {
      if (e.weight > 0) {
        total += Math.abs(e.weight);
      }
    }

    if (total != 0) {
      for (const e of edges) {
        const frac = Math.abs(e.weight) / total;
        score += -frac * Math.log2(frac);
      }
      if (self > 0) {
        const frac = Math.abs(self) / total;
        score += -frac * Math.log2(frac);
      }
    }
    return score;
  }

  const cachedScores = {};
  for (const key in graphNodes) {
    cachedScores[graphNodes[key].name] = entropyScore(graphNodes[key]);
  }

  const sortedNodes = Object.values(graphNodes).sort((a, b) => {
    const sa = cachedScores[a.name];
    const sb = cachedScores[b.name];
    if (sa !== sb) {
      return sb - sa;
    }
    if (a.name !== b.name) {
      return a.name < b.name ? -1 : 1;
    }
    if (a.self !== b.self) {
      return sb - sa;
    }

    return a.name < b.name ? -1 : 1;
  });

  const keptNodes = {};
  for (const n of sortedNodes) {
    keptNodes[n.name] = n;
  }

  sortedNodes.slice(maxNodes).forEach((n) => {
    delete keptNodes[n.name];
  });

  // now that we removed nodes we need to create residual edges
  function trimTree(n: TreeNode, lastPresentParent: TreeNode | null) {
    const isNodeDeleted = !keptNodes[n.name];
    for (const child of n.children) {
      const isChildNodeDeleted = !keptNodes[child.name];
      trimTree(child, isNodeDeleted ? lastPresentParent : n);
      if (!isChildNodeDeleted && lastPresentParent && isNodeDeleted) {
        const edgeKey = lastPresentParent.name + '/' + child.name;
        graphEdges[edgeKey] ||= {
          from: graphNodes[lastPresentParent.name],
          to: graphNodes[child.name],
          weight: 0,
          residual: true,
        };

        graphEdges[edgeKey].weight += child.total[0];
        graphEdges[edgeKey].residual = true;
      }
    }
  }

  trimTree(tree, null);

  graphNodes = keptNodes;

  function isRedundantEdge(e: GraphEdge) {
    const [src, n] = [e.from, e.to];
    const seen = {};
    const queue = [n];

    while (queue.length > 0) {
      const n = queue.shift() as GraphNode;
      for (const ie of n.parents) {
        if (e == ie || seen[ie.from.name]) {
          continue;
        }
        if (ie.from == src) {
          return true;
        }
        seen[ie.from.name] = true;
        queue.push(ie.from);
      }
    }
    return false;
  }

  // remove redundant edges
  console.log('remove redundant edges');
  for (const node of sortedNodes.reverse()) {
    // node.children
    const sortedParentEdges = node.parents.sort((a, b) => b.weight - a.weight);
    const edgesToDelete: GraphEdge[] = [];
    for (const parentEdge of sortedParentEdges) {
      if (!parentEdge.residual) {
        break;
      }

      if (isRedundantEdge(parentEdge)) {
        console.log('delete', parentEdge.from.name + '/' + parentEdge.to.name);
        edgesToDelete.push(parentEdge);
        delete graphEdges[parentEdge.from.name + '/' + parentEdge.to.name];
      }
    }
    for (const edge of edgesToDelete) {
      edge.from.children = edge.from.children.filter((e) => e.to != edge.to);
      edge.to.parents = edge.to.parents.filter((e) => e.from != edge.from);
    }

    // in := n.In.Sort()
    // for j := len(in); j > 0; j-- {
    //   e := in[j-1]
    //   if !e.Residual {
    //     // Do not remove edges heavier than a non-residual edge, to
    //     // avoid potential confusion.
    //     break
    //   }
    //   if isRedundantEdge(e) {
    //     delete(e.Src.Out, e.Dest)
    //     delete(e.Dest.In, e.Src)
    //   }
    // }
  }

  // now we clean up edges
  for (const key in graphEdges) {
    const e = graphEdges[key];
    // first delete the ones that no longer have nodes
    if (!graphNodes[e.from.name]) {
      delete graphEdges[key];
    }
    if (!graphNodes[e.to.name]) {
      delete graphEdges[key];
    }
    // second delete the ones that are too small
    if (e.weight < edgeCutoff) {
      delete graphEdges[key];
    }
  }

  for (const key in graphNodes) {
    nodes.push(renderNode(formatFunc, graphNodes[key], maxSelf, maxTotal));
  }

  for (const key in graphEdges) {
    edges.push(renderEdge(formatFunc, graphEdges[key], maxTotal));
  }

  return `digraph "unnamed" {
    fontname= "${fontName}"
    node [style=filled fillcolor="#f8f8f8"]
    ${nodes.join('\n')}
    ${edges.join('\n')}
  }`;
}
