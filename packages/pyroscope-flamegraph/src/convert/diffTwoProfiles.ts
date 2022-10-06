/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck

import type { Profile } from '@pyroscope/models/src';
import {
  deltaDiffWrapper,
  deltaDiffWrapperReverse,
} from '../FlameGraph/decode';

export function flamebearersToTree(f1: Flamebearer, f2: Flamebearer) {
  const lookup = {};
  const lookup2 = {};
  let root;
  [f1, f2].forEach((f, fi) => {
    for (let i = 0; i < f.levels.length; i += 1) {
      for (let j = 0; j < f.levels[i].length; j += 4) {
        const key2 = [fi, i, j].join('/');
        const name = f.names[f.levels[i][j + 3]];
        const offset = f.levels[i][j + 0] as number;

        let parentKey;
        if (i !== 0) {
          const pi = i - 1;
          for (let k = 0; k < f.levels[pi].length; k += 4) {
            const parentOffset = f.levels[pi][k + 0] as number;
            const total = f.levels[pi][k + 1] as number;
            if (offset >= parentOffset && offset < parentOffset + total) {
              const parentKey2 = [fi, pi, k].join('/');
              const parentObj = lookup2[parentKey2];
              parentKey = parentObj.key;
              break;
            }
          }
        }

        const key = [parentKey || '', name].join('/');
        lookup[key] ||= {
          name,
          children: [],
          self: [],
          total: [],
          key,
        };
        const obj = lookup[key];
        obj.total[fi] ||= 0;
        obj.total[fi] += f.levels[i][j + 1];
        obj.self[fi] ||= 0;
        obj.self[fi] += f.levels[i][j + 2];
        lookup2[key2] = obj;
        if (parentKey) {
          lookup[parentKey].children.push(obj);
        }
        if (i === 0) {
          root = obj;
        }
      }
    }
  });

  return root;
}

function diffFlamebearer(f1: Flamebearer, f2: Flamebearer): Flamebearer {
  const result: Flamebearer = {
    format: 'double',
    numTicks: (f1.numTicks as number) + (f2.numTicks as number),
    leftTicks: f1.numTicks,
    rightTicks: f2.numTicks,
    maxSelf: 100,
    sampleRate: f1.sampleRate,
    names: [],
    levels: [],
    units: f1.units,
    spyName: f1.spyName,
  };

  const tree = flamebearersToTree(f1, f2);
  const processNode = (
    node,
    level: number,
    offsetLeft: number,
    offsetRight: number
  ) => {
    const { name, children, self, total } = node;
    result.names.push(name);
    result.levels[level] ||= [];
    result.maxSelf = Math.max(result.maxSelf, self[0] || 0, self[1] || 0);
    result.levels[level] = result.levels[level].concat([
      offsetLeft,
      total[0] || 0,
      self[0] || 0,
      offsetRight,
      total[1] || 0,
      self[1] || 0,
      result.names.length - 1,
    ]);
    for (let i = 0; i < children.length; i += 1) {
      const [ol, or] = processNode(
        children[i],
        level + 1,
        offsetLeft,
        offsetRight
      );
      offsetLeft += ol;
      offsetRight += or;
    }
    return [total[0] || 0, total[1] || 0];
  };

  processNode(tree, 0, 0, 0);

  return result;
}

export function diffTwoProfiles(p1: Profile, p2: Profile): Profile {
  p1.flamebearer.levels = deltaDiffWrapper('single', p1.flamebearer.levels);
  p2.flamebearer.levels = deltaDiffWrapper('single', p2.flamebearer.levels);
  const resultFlamebearer = diffFlamebearer(p1.flamebearer, p2.flamebearer);
  resultFlamebearer.levels = deltaDiffWrapperReverse(
    'double',
    resultFlamebearer.levels
  );
  const metadata = { ...p1.metadata };
  metadata.format = 'double';
  return {
    version: 1,
    flamebearer: resultFlamebearer,
    metadata,
    leftTicks: p1.flamebearer.numTicks,
    rightTicks: p2.flamebearer.numTicks,
  };
}
