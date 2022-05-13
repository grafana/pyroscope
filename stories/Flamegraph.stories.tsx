import React, { useState } from 'react';
import { FlamegraphRenderer, Box } from '@pyroscope/flamegraph';
import PyroscopeServerCPU from '../cypress/fixtures/pyroscope.server.cpu.json';
import SimpleGolangCPU from '../cypress/fixtures/simple-golang-app-cpu.json';
import Button from '@ui/Button';
import { ComponentStory } from '@storybook/react';

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

const Template: ComponentStory<typeof Button> = (args) => (
  <FlamegraphRenderer
    flamebearer={SimpleTree}
    display="flamegraph"
    viewType="single"
    colorMode={args['Light Color Mode'] ? 'light' : 'dark'}
  />
);

export const ColorMode = Template.bind({});

ColorMode.args = {
  ['Light Color Mode']: false,
};

export const WithToolbar = () => {
  return (
    <Box>
      <FlamegraphRenderer flamebearer={SimpleTree} />
    </Box>
  );
};

export const WithoutToolbar = () => {
  return (
    <Box>
      <FlamegraphRenderer flamebearer={SimpleTree} showToolbar={false} />
    </Box>
  );
};

export const JustFlamegraph = () => {
  return (
    <FlamegraphRenderer
      flamebearer={SimpleTree}
      onlyDisplay="flamegraph"
      showToolbar={false}
    />
  );
};

// In this case having the toolbar doesn't make much sense?
export const TableViewWithoutToolbar = () => {
  return (
    <Box>
      <FlamegraphRenderer
        flamebearer={SimpleTree}
        onlyDisplay="table"
        showToolbar={false}
      />
    </Box>
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
      <Box>
        <FlamegraphRenderer profile={profile} showToolbar={false} />
      </Box>
    </>
  );
};
