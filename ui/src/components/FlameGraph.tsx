import { useMemo } from 'react';
import { FlameGraph as GrafanaFlameGraph } from '@grafana/flamegraph';
import { createTheme, FieldType } from '@grafana/data';
import type { DataFrame } from '@grafana/data';
import type { FlamegraphData } from '@api/client';
import { profileTypeUnit } from '@api/client';
import { Empty } from '@components/core/Empty';
import './FlameGraph.css';

function toGrafanaUnit(unit: string): string {
  switch (unit) {
    case 'ns':
      return 'ns';
    case 'bytes':
      return 'bytes';
    default:
      return 'short';
  }
}

interface ParsedNode {
  name: string;
  start: number;
  end: number;
  self: number;
  level: number;
  children: ParsedNode[];
}

function toDataFrame(
  data: FlamegraphData,
  unit: string,
): DataFrame | undefined {
  const { names, levels } = data;
  if (!levels.length) return undefined;

  const nodesByLevel: ParsedNode[][] = levels.map((rawLevel, li) => {
    const nodes: ParsedNode[] = [];
    let pos = 0;
    const { values } = rawLevel;
    for (let i = 0; i + 3 < values.length; i += 4) {
      const offset = parseInt(values[i], 10);
      const total = parseInt(values[i + 1], 10);
      const self = parseInt(values[i + 2], 10);
      const nameIdx = parseInt(values[i + 3], 10);
      pos += offset;
      if (total > 0) {
        nodes.push({
          name: names[nameIdx] ?? '',
          start: pos,
          end: pos + total,
          self,
          level: li,
          children: [],
        });
      }
      pos += total;
    }
    return nodes;
  });

  for (let li = 1; li < nodesByLevel.length; li++) {
    const parents = nodesByLevel[li - 1];
    let pi = 0;
    for (const node of nodesByLevel[li]) {
      while (pi < parents.length - 1 && parents[pi].end <= node.start) pi++;
      if (
        pi < parents.length &&
        parents[pi].start <= node.start &&
        node.end <= parents[pi].end
      ) {
        parents[pi].children.push(node);
      }
    }
  }

  const labelVals: string[] = [];
  const levelVals: number[] = [];
  const valueVals: number[] = [];
  const selfVals: number[] = [];

  function dfs(node: ParsedNode) {
    labelVals.push(node.name);
    levelVals.push(node.level);
    valueVals.push(node.end - node.start);
    selfVals.push(node.self);
    for (const child of node.children) dfs(child);
  }

  for (const root of nodesByLevel[0]) dfs(root);

  if (labelVals.length === 0) return undefined;

  return {
    name: 'flamegraph',
    refId: 'A',
    fields: [
      { name: 'level', values: levelVals, type: FieldType.number, config: {} },
      {
        name: 'value',
        values: valueVals,
        type: FieldType.number,
        config: { unit },
      },
      {
        name: 'self',
        values: selfVals,
        type: FieldType.number,
        config: { unit },
      },
      { name: 'label', values: labelVals, type: FieldType.string, config: {} },
    ],
    length: labelVals.length,
  } as unknown as DataFrame;
}

export function FlameGraph({
  data,
  theme,
  profileTypeId,
}: {
  data: FlamegraphData;
  theme: 'dark' | 'light';
  profileTypeId: string;
}) {
  const unit = toGrafanaUnit(profileTypeUnit(profileTypeId));
  const dataFrame = useMemo(() => toDataFrame(data, unit), [data, unit]);
  const getTheme = useMemo(
    () => () => createTheme({ colors: { mode: theme } }),
    [theme],
  );

  if (!dataFrame) {
    return <Empty />;
  }

  return (
    <div className="flamegraph-wrapper">
      <GrafanaFlameGraph data={dataFrame} getTheme={getTheme} />
    </div>
  );
}
