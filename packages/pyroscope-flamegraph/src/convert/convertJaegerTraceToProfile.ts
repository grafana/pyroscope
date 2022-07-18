/* eslint-disable import/prefer-default-export */
import groupBy from 'lodash.groupby';
import map from 'lodash.map';
import type { Profile, Trace, TraceSpan } from '@pyroscope/models/src';

interface Span extends TraceSpan {
  children: Span[];
  total: number;
  self: number;
}

// TODO: need to remove this ideally
function deltaDiffWrapperReverse(
  format: Profile['metadata']['format'],
  levels: Profile['flamebearer']['levels']
) {
  const mutableLevels = [...levels];

  function deltaDiff(
    lvls: Profile['flamebearer']['levels'],
    start: number,
    step: number
  ) {
    // eslint-disable-next-line no-restricted-syntax
    for (const level of lvls) {
      let total = 0;
      for (let i = start; i < level.length; i += step) {
        level[i] -= total;
        total += level[i] + level[i + 1];
      }
    }
  }

  if (format === 'double') {
    deltaDiff(mutableLevels, 0, 7);
    deltaDiff(mutableLevels, 3, 7);
  } else {
    deltaDiff(mutableLevels, 0, 4);
  }

  return mutableLevels;
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

  // hack, need to remove this ideally
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
