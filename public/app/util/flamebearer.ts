import {
  DataFrameDTO,
  FieldType,
  createDataFrame,
} from '@grafana/data';

export function deltaDiffWrapper(format: 'single' | 'double', levels: number[][]) {
  const mutable_levels = [...levels];

  function deltaDiff(levels: number[][], start: number, step: number) {
    for (const level of levels) {
      let prev = 0;
      for (let i = start; i < level.length; i += step) {
        level[i] += prev;
        prev = level[i] + level[i + 1];
      }
    }
  }

  if (format === 'double') {
    deltaDiff(mutable_levels, 0, 7);
    deltaDiff(mutable_levels, 3, 7);
  } else {
    deltaDiff(mutable_levels, 0, 4);
  }

  return mutable_levels;
}

function getNodes(level: number[], names: string[]) {
  const nodes = [];
  for (let i = 0; i < level.length; i += 4) {
    nodes.push({
      level: 0,
      label: names[level[i + 3]],
      val: level[i + 1],
      self: level[i + 2],
      offset: level[i],
      children: [],
    });
  }
  return nodes;
}

export function diffFlamebearerToDataFrameDTO(levels: number[][], names: string[]) {
  const nodeLevels: any[][] = [];
  for (let i = 0; i < levels.length; i++) {
    nodeLevels[i] = [];
    for (const node of getNodes(levels[i], names)) {
      node.level = i;
      nodeLevels[i].push(node);
      if (i > 0) {
        const prevNodesInLevel = nodeLevels[i].slice(0, -1);
        const currentNodeStart =
          prevNodesInLevel.reduce(
            (acc: number, n: any) => n.offset + n.val + acc,
            0
          ) + node.offset;

        const prevLevel = nodeLevels[i - 1];
        let prevLevelOffset = 0;
        for (const prevLevelNode of prevLevel) {
          const parentNodeStart = prevLevelOffset + prevLevelNode.offset;
          const parentNodeEnd = parentNodeStart + prevLevelNode.val;

          if (
            parentNodeStart <= currentNodeStart &&
            parentNodeEnd > currentNodeStart
          ) {
            prevLevelNode.children.push(node);
            break;
          } else {
            prevLevelOffset += prevLevelNode.offset + prevLevelNode.val;
          }
        }
      }
    }
  }

  const root = nodeLevels[0][0];
  const stack = [root];

  const labelValues = [];
  const levelValues = [];
  const selfValues = [];
  const valueValues = [];

  while (stack.length) {
    const node = stack.shift();
    labelValues.push(node.label);
    levelValues.push(node.level);
    selfValues.push(node.self);
    valueValues.push(node.val);
    stack.unshift(...node.children);
  }

  const frame: DataFrameDTO = {
    name: 'response',
    meta: { preferredVisualisationType: 'flamegraph' },
    fields: [
      { name: 'level', values: levelValues },
      { name: 'label', values: labelValues, type: FieldType.string },
      { name: 'self', values: selfValues },
      { name: 'value', values: valueValues },
    ],
  };

  return createDataFrame(frame);
}
