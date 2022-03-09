import React, { useState } from 'react';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import PyroscopeServerCPU from '../cypress/fixtures/pyroscope.server.cpu.json';
import SimpleGolangCPU from '../cypress/fixtures/simple-golang-app-cpu.json';
import Button from '@ui/Button';

export default {
  title: '@pyroscope/flamegraph',
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
  units: 'samples',
  fitMode: 'HEAD',

  spyName: 'gospy',
};

export const WithToolbar = () => {
  return (
    <FlamegraphRenderer
      flamebearer={SimpleTree}
      display="flamegraph"
      viewType="single"
    />
  );
};

export const WithoutToolbar = () => {
  return (
    <FlamegraphRenderer
      flamebearer={SimpleTree}
      viewType="single"
      display="flamegraph"
      showToolbar={false}
    />
  );
};

// In this case having the toolbar doesn't make much sense?
export const TableViewWithoutToolbar = () => {
  return (
    <FlamegraphRenderer
      flamebearer={SimpleTree}
      viewType="single"
      display="table"
      showToolbar={false}
    />
  );
};

// In this example we use the FlamegraphRenderer component
// with whatever data we got from the /render endpoint
export const WithRenderData = () => {
  const [profile, setProfile] = useState(PyroscopeServerCPU);
  return (
    <>
      <Button onClick={() => setProfile(SimpleGolangCPU)}>Simple Tree</Button>
      <Button onClick={() => setProfile(PyroscopeServerCPU)}>
        Complex Tree
      </Button>
      <FlamegraphRenderer
        profile={profile}
        viewType="single"
        display="flamegraph"
        showToolbar={false}
      />
    </>
  );
};
