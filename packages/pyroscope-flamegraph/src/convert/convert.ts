import groupBy from 'lodash.groupby';
import map from 'lodash.map';

export function traceToFlamebearer(trace: any) {
  let result = {
    topLevel: 0,
    rangeMin: 0,
    format: 'single' as const,
    numTicks: 0,
    sampleRate: 1000000,
    names: [],
    levels: [],

    rangeMax: 1,
    units: 'samples',
    fitMode: 'HEAD',

    spyName: 'tracing',
  };

  // Step 1: converting spans to a tree
  var spans = {};
  var root = { children: [] };
  trace.spans.forEach((span) => {
    span.children = [];
    spans[span.spanID] = span;
  });

  trace.spans.forEach((span) => {
    let node = root;
    if (span.references && span.references.length > 0) {
      node = spans[span.references[0].spanID] || root;
    }
    // @ts-ignore
    node.children.push(span);
  });

  // Step 2: group spans with same name

  // @ts-ignore
  function groupSpans(span: any, d: number) {
    (span.children || []).forEach((x) => groupSpans(x, d + 1));

    let childrenDur = 0;
    const groups = groupBy(span.children || [], (x) => x.operationName);
    span.children = map(groups, (group) => {
      let res = group[0];
      for (let i = 1; i < group.length; i += 1) {
        res.duration += group[i].duration;
      }
      childrenDur += res.duration;
      return res;
    });
    span.total = span.duration || childrenDur;
    span.self = Math.max(0, span.total - childrenDur);
  }
  groupSpans(root, 0);

  // Step 3: traversing the tree

  // @ts-ignore
  function processNode(span: any, level: number, offset: number) {
    result.numTicks ||= span.total;
    // @ts-ignore
    result.levels[level] ||= [];
    // @ts-ignore
    result.levels[level].push(offset);
    // @ts-ignore
    result.levels[level].push(span.total);
    // @ts-ignore
    result.levels[level].push(span.self);
    result.names.push(
      // @ts-ignore
      (span.processID
        ? trace.processes[span.processID].serviceName + ': '
        : '') + (span.operationName || 'total')
    );
    // @ts-ignore
    result.levels[level].push(result.names.length - 1);

    (span.children || []).forEach((x) => {
      offset += processNode(x, level + 1, offset);
    });
    return span.total;
  }

  processNode(root, 0, 0);

  return result;
}
