/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck

import type { Flamebearer } from '@pyroscope/models/src';

export function flamebearersToTree(f1: Flamebearer, f2?: Flamebearer) {
  const lookup = {};
  const lookup2 = {};
  let root;
  (f2 ? [f1, f2] : [f1]).forEach((f, fi) => {
    for (let i = 0; i < f.levels.length; i += 1) {
      for (let j = 0; j < f.levels[i].length; j += 4) {
        const key2 = [fi, i, j].join('/');
        const name = f.names[f.levels[i][j + 3]];
        const offset = f.levels[i][j + 0];

        let parentKey;
        if (i !== 0) {
          const pi = i - 1;
          for (let k = 0; k < f.levels[pi].length; k += 4) {
            const parentOffset = f.levels[pi][k + 0];
            const total = f.levels[pi][k + 1];
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
