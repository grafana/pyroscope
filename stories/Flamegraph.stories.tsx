import React from 'react';
import { Units } from '@utils/format';
import { Option } from 'prelude-ts';
import Flamegraph from '../webapp/javascript/components/FlameGraph/FlameGraphRenderer';

export default {
  title: 'Flamegraph',
};

const SimpleTree = {
  topLevel: 0,
  rangeMin: 0,
  format: 'single' as const,
  numTicks: 988,
  sampleRate: 100,
  names: [
    'total',
    'runtime.main',
    'main.slowFunction',
    'main.work',
    'main.main',
    'main.fastFunction',
  ],
  levels: [
    [0, 988, 0, 0],
    [0, 988, 0, 1],
    [0, 214, 0, 5, 214, 3, 2, 4, 217, 771, 0, 2],
    [0, 214, 214, 3, 216, 1, 1, 5, 217, 771, 771, 3],
  ],

  rangeMax: 1,
  units: Units.Samples,
  fitMode: 'HEAD',

  spyName: 'gospy',
};

export const WithToolbar = () => {
  return <Flamegraph flamebearer={SimpleTree} viewType="grafana" />;
};

export const WithoutToolbar = () => {
  return (
    <Flamegraph
      flamebearer={SimpleTree}
      viewType="grafana"
      showToolbar={false}
    />
  );
};
