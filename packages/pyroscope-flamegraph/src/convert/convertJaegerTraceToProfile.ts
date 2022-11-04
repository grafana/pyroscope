/* eslint-disable import/prefer-default-export */
import groupBy from 'lodash.groupby';
import map from 'lodash.map';
import type { Profile, Trace, TraceSpan } from '@pyroscope/models/src';
import { deltaDiffWrapperReverse } from '../FlameGraph/decode';

interface Span extends TraceSpan {
  children: Span[];
  total: number;
  self: number;
}

export function convertJaegerTraceToProfile(trace: Trace): Profile {
  const resultFlamebearer = {
    numTicks: 0,
    maxSelf: 0,
    names: [] as string[],
    levels: [] as number[][],
  };

  // Step 1: converting spans to a tree

  const spans: Record<string, Span> = {};
  const root: Span = { children: [] } as unknown as Span;
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
      const res = group[0];
      for (let i = 1; i < group.length; i += 1) {
        res.total += group[i].total;
      }
      childrenDur += res.total;
      return res;
    });
    span.total = Math.max(span.duration, childrenDur);
    span.self = Math.max(0, span.total - childrenDur);
  }
  groupSpans(root, 0);

  // Step 3: traversing the tree

  function processNode(span: Span, level: number, offset: number) {
    resultFlamebearer.numTicks ||= span.total;
    resultFlamebearer.levels[level] ||= [];
    resultFlamebearer.levels[level].push(offset);
    resultFlamebearer.levels[level].push(span.total);
    resultFlamebearer.levels[level].push(span.self);
    resultFlamebearer.names.push(
      (span.processID
        ? `${trace.processes[span.processID].serviceName}: `
        : '') + (span.operationName || 'total')
    );
    resultFlamebearer.levels[level].push(resultFlamebearer.names.length - 1);

    (span.children || []).forEach((x) => {
      offset += processNode(x, level + 1, offset);
    });
    return span.total;
  }

  processNode(root, 0, 0);

  resultFlamebearer.levels = deltaDiffWrapperReverse(
    'single',
    resultFlamebearer.levels
  );

  return {
    version: 1,
    flamebearer: resultFlamebearer,
    metadata: {
      format: 'single',
      units: 'trace_samples',
      spyName: 'tracing',
      sampleRate: 1000000,
    },
  };
}
