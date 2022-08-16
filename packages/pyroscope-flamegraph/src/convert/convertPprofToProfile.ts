/* eslint-disable import/prefer-default-export */
import type { Profile } from '@pyroscope/models/src';

// import {Profile as PProfProfile} from 'pprof/proto';

import { perftools } from 'pprof/proto/profile';
import { string } from 'zod';
import { deltaDiffWrapperReverse } from '../FlameGraph/decode';

type node = {
  name: string;
  value: number;
  total: number;
  children: object;
};

export function convertPprofToProfile(
  pprofBuf: Uint8Array,
  selectedSampleType: string
): Profile {
  const resultFlamebearer = {
    numTicks: 0,
    maxSelf: 0,
    names: [] as string[],
    levels: [] as number[][],
  };

  const pprofProfile = perftools.profiles.Profile.decode(pprofBuf);

  let sampleTypeIndex = -1;
  for (let i = 0; i < pprofProfile.sampleType.length; i++) {
    if (
      pprofProfile.stringTable[pprofProfile.sampleType[i].type] ===
      selectedSampleType
    ) {
      sampleTypeIndex = i;
      break;
    }
  }

  if (sampleTypeIndex === -1) {
    throw 'sample type is not found in the profile';
  }

  let root: node = {
    name: 'total',
    value: 0,
    total: 0,
    children: {},
  };

  function findObjectByID(obj, id: number) {
    return obj.find((x) => x.id === id);
  }

  for (let i = 0; i < pprofProfile.sample.length; i++) {
    let currentNode = root;
    const sample = pprofProfile.sample[i];
    let lastNode;
    if (!sample.locationId) {
      continue;
    }
    if (!sample.value) {
      continue;
    }
    for (let j = sample.locationId.length - 1; j >= 0; j--) {
      const location = findObjectByID(
        pprofProfile.location,
        sample.locationId[j]
      );
      if (!location || !location.line) {
        continue;
      }
      for (let k = location.line.length - 1; k >= 0; k--) {
        const line = location.line[k];
        if (!line) {
          continue;
        }
        const func = findObjectByID(pprofProfile['function'], line.functionId);
        if (!func) {
          continue;
        }
        const name = pprofProfile.stringTable[func.name];
        const childNode = currentNode.children[name] || {
          children: {},
          name,
          value: 0,
          total: 0,
        };
        currentNode.children[name] = childNode;
        currentNode = childNode;
        lastNode = childNode;
      }
    }
    lastNode.value += sample.value[sampleTypeIndex];
  }

  function populateTotal(node: node): number {
    node.total += node.value;
    (Object.values(node.children) || []).forEach((x) => {
      node.total += populateTotal(x);
    });
    return node.total;
  }

  function processNode(node: node, level: number, offset: number): number {
    resultFlamebearer.levels[level] ||= [];
    resultFlamebearer.numTicks ||= node.total;
    resultFlamebearer.levels[level].push(offset);
    resultFlamebearer.levels[level].push(node.total);
    resultFlamebearer.levels[level].push(node.value);
    resultFlamebearer.names.push(node.name);
    resultFlamebearer.levels[level].push(resultFlamebearer.names.length - 1);
    resultFlamebearer.maxSelf = Math.max(node.value, resultFlamebearer.maxSelf);

    (Object.values(node.children) || []).forEach((x) => {
      offset += processNode(x, level + 1, offset);
    });

    return node.total;
  }

  populateTotal(root);
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
      units: 'samples',
      spyName: 'gospy',
      sampleRate: 100,
    },
  };
}
