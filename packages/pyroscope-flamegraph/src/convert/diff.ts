/* eslint-disable import/prefer-default-export */
import groupBy from 'lodash.groupby';
import map from 'lodash.map';
import type { Flamebearer } from '@pyroscope/models/src';

export function diffFlamebearer(f1: Flamebearer, f2: Flamebearer): Flamebearer {
  const result: Flamebearer = {
    format: 'double',
    numTicks: 0,
    leftTicks: f1.numTicks,
    rightTicks: f2.numTicks,
    maxSelf: 0,
    sampleRate: 1000000,
    names: [],
    levels: [],
    units: f1.units,
    spyName: f1.spyName,
  };

  return f1;
  // return result;
}
