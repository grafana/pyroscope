import { Profile } from 'packages/pyroscope-models/src';

import decodeFlamebearer from '../FlameGraph/decode';
import { flamebearersToTree, TreeNode } from './flamebearersToTree';
import { DurationFormatter } from '../format/format';

function renderLabels(obj) {
  const labels: string[] = [];
  for (const key in obj) {
    labels.push(`${key}="${escapeForDot(String(obj[key] || ''))}"`);
  }
  return `[${labels.join(' ')}]`;
}

const baseFontSize = 8;
const maxFontGrowth = 16;

function formatPercent(a, b) {
  return ((a * 100) / b).toFixed(2) + '%';
}

function formatDuration(milliseconds) {
  return `${milliseconds}ms`;
}

// TODO: styling
// TODO: cut the tree
// TODO: making it look more pyroscopy

type durFormatter = (dur: number) => string;

function renderNode(
  durFormatter: durFormatter,
  n: TreeNode,
  index: number,
  maxSelf: number,
  maxTotal: number
): string {
  const self = n.self[0];
  const total = n.total[0];

  const name = n.name.replace(/"/g, '\\"');
  const dur = durFormatter(self);
  // const percent = formatPercent(self, maxSelf);
  const fontsize =
    baseFontSize + Math.ceil(maxFontGrowth * Math.sqrt(self / maxSelf));
  const color = '#b23100'; // TODO
  const fillcolor = '#eddbd5'; // TODO

  const label = formatNodeLabel(name, self, total, maxSelf);
  console.log(label);

  const labels = {
    label: label,
    id: `node${index}`,
    fontsize: fontsize,
    shape: 'box',
    tooltip: `${name} (${dur})`,
    color: color, // TODO
    fillcolor: fillcolor, // TODO
  };
  return `N${index} ${renderLabels(labels)}`;
}

function escapeForDot(str: string) {
  return str.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

// Removes package name and method arguments for Java method names.
// See tests for examples.
const javaRegExp = new RegExp(
  '^(?:[a-z]\\w*\\.)*([A-Z][\\w$]*\\.(?:<init>|[a-z][\\w$]*(?:\\$\\d+)?))(?:(?:\\()|$)'
);
// Removes package name and method arguments for Go function names.
// See tests for examples.
const goRegExp = new RegExp('^(?:[\\w\\-\\.]+\\/)+(.+)');
// Removes potential module versions in a package path.
const goVerRegExp = new RegExp('^(.*?)/v(?:[2-9]|[1-9][0-9]+)([./].*)$');
// Strips C++ namespace prefix from a C++ function / method name.
// NOTE: Make sure to keep the template parameters in the name. Normally,
// template parameters are stripped from the C++ names but when
// -symbolize=demangle=templates flag is used, they will not be.
// See tests for examples.
const cppRegExp = new RegExp(
  '^(?:[_a-zA-Z]\\w*::)+(_*[A-Z]\\w*::~?[_a-zA-Z]\\w*(?:<.*>)?)'
);
const cppAnonymousPrefixRegExp = new RegExp('^\\(anonymous namespace\\)::');

function shortenFunctionName(f) {
  f = f.replace(cppAnonymousPrefixRegExp, '');
  f = f.replace(goVerRegExp, '${1}${2}');
  for (let re of [goRegExp, javaRegExp, cppRegExp]) {
    let matches = f.match(re);
    if (matches && matches.length >= 2) {
      return matches.slice(1).join('');
    }
  }
  return f;
}

function pathBasename(p) {
  return p.replace(/.*\//, '');
}

function multilinePrintableName(name) {
  // var infoCopy = Object.assign({}, info);
  // name = escapeForDot(shortenFunctionName(name));
  // name = name.replace(/::/g, '\n');
  // // Go type parameters are reported as "[...]" by Go pprof profiles.
  // // Keep this ellipsis rather than replacing with newlines below.
  // name = name.replace(/\[...\]/g, '[…]');
  // name = name.replace(/\./g, '\n');
  // if (infoCopy.File !== '') {
  // 	infoCopy.File = pathBasename(infoCopy.File);
  // }
  // return infoCopy.NameComponents().join('\n') + '\n';
}

function formatNodeLabel(name, self, total, maxTotal) {
  var label: string = '';
  // TODO: split package name and name
  // label = multilinePrintableName(node.Info);
  label = pathBasename(name) + '\n';

  var selfValue = formatDuration(self);
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
    totalValue = formatDuration(total);
    label =
      label + 'of ' + totalValue + ' (' + formatPercent(total, maxTotal) + ')';
  }

  return label;
}

function renderEdge(
  durFormatter: durFormatter,
  src: TreeNode,
  dst: TreeNode,
  srcIndex: number,
  dstIndex: number,
  total: number
): string {
  const srcName = src.name.replace(/"/g, '\\"');
  const dstName = dst.name.replace(/"/g, '\\"');
  const dur = durFormatter(dst.total[0]);
  const edgeWeight = dst.total[0]; // TODO
  const weight = 1 + (edgeWeight * 100) / total;
  const penwidth = 1 + (edgeWeight * 5) / total;
  const color = '#b2ac9f'; // TODO
  const tooltip = `${srcName} -> ${dstName} (${dur})`;

  const labels = {
    label: dur,
    weight: weight,
    penwidth: penwidth,
    color: color,
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

  function calcMaxValues(
    n: TreeNode,
    maxSelf: number,
    maxTotal: number
  ): [number, number] {
    for (const child of n.children) {
      const [newMaxSelf, newMaxTotal] = calcMaxValues(child, maxSelf, maxTotal);
      maxSelf = Math.max(maxSelf, newMaxSelf);
      maxTotal = Math.max(maxTotal, newMaxTotal);
    }

    maxSelf = Math.max(maxSelf, n.self[0]);
    maxTotal = Math.max(maxTotal, n.total[0]);

    return [maxSelf, maxTotal];
  }

  const [maxSelf, maxTotal] = calcMaxValues(tree, 0, 0);
  const { sampleRate } = p.metadata;
  const durationFormatter = new DurationFormatter(maxTotal / sampleRate);

  const durFormatter = (dur: number): string => {
    return durationFormatter.format(dur, sampleRate, false);
  };

  function processNode(n: TreeNode): number {
    const srcIndex = nodes.length;
    nodes.push(renderNode(durFormatter, n, srcIndex, maxSelf, maxTotal));
    for (const child of n.children) {
      const dstIndex = processNode(child);
      const total = n.total[0];
      edges.push(renderEdge(durFormatter, n, child, srcIndex, dstIndex, total));
    }
    return srcIndex;
  }

  processNode(tree);

  return `digraph "unnamed" {
    ${nodes.join('\n')}
    ${edges.join('\n')}
  }`;
}
