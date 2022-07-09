/* eslint-disable import/prefer-default-export */
import groupBy from 'lodash.groupby';
import map from 'lodash.map';
import type { Flamebearer, Trace, TraceSpan } from '@pyroscope/models';

interface Span extends TraceSpan {
  children: Span[];
  total: number;
  self: number;
}

export function traceToFlamebearer(trace: Trace): Flamebearer {
  let result: Flamebearer = {
    format: 'single',
    numTicks: 0,
    sampleRate: 1000000,
    names: [],
    levels: [],
    units: 'trace_samples',
    spyName: 'tracing',
  };

  // Step 1: converting spans to a tree

  var spans: Record<string, Span> = {};
  var root: Span = { children: [] } as unknown as Span;
  (trace.spans as Span[]).forEach((span) => {
    span.children = [];
    spans[span.spanID] = span;
  });

  (trace.spans as Span[]).forEach((span) => {
    let node = root;
    if (span.references && span.references.length > 0) {
      node = spans[span.references[0].spanID] || root;
    }

    node.children.push(span);
  });

  // Step 2: group spans with same name

  function groupSpans(span: Span, d: number) {
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

  function processNode(span: Span, level: number, offset: number) {
    result.numTicks ||= span.total;
    result.levels[level] ||= [];
    result.levels[level].push(offset);
    result.levels[level].push(span.total);
    result.levels[level].push(span.self);
    result.names.push(
      (span.processID
        ? trace.processes[span.processID].serviceName + ': '
        : '') + (span.operationName || 'total')
    );
    result.levels[level].push(result.names.length - 1);

    (span.children || []).forEach((x) => {
      offset += processNode(x, level + 1, offset);
    });
    return span.total;
  }

  processNode(root, 0, 0);

  return result;
}
