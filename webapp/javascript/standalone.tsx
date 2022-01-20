import ReactDOM from 'react-dom';
import React from 'react';
import Box from '@ui/Box';
import { decodeFlamebearer } from '@models/flamebearer';
import FlameGraphRenderer from './components/FlameGraph/FlameGraphRenderer';
import styles from './standalone.module.scss';

// just an example
const defaultFlamegraph = {
  flamebearer: {
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
      [0, 214, 0, 5, 0, 3, 2, 4, 0, 771, 0, 2],
      [0, 214, 214, 3, 2, 1, 1, 5, 0, 771, 771, 3],
    ],
    numTicks: 988,
    maxSelf: 771,
    spyName: 'gospy',
    sampleRate: 100,
    units: 'samples',
    format: 'single',
  },
  metadata: {
    format: 'single',
    sampleRate: 100,
    spyName: 'gospy',
    units: 'samples',
  },
  timeline: {
    startTime: 1632335270,
    samples: [989],
    durationDelta: 10,
  },
};

// Enable this if you are developing and don't want to run a server
// (window as any).flamegraph = defaultFlamegraph;

if (!(window as any).flamegraph) {
  alert(`'flamegraph' is required`);
  throw new Error(`'flamegraph' is required`);
}

// TODO parse window.flamegraph
const { flamegraph } = window as any;

function AdhocApp() {
  const flamebearer = decodeFlamebearer(flamegraph);

  return (
    <Box className={styles.container}>
      <FlameGraphRenderer
        flamebearer={flamebearer}
        viewType="single"
        display="both"
        rawFlamegraph={flamegraph}
        ExportData={<div />}
      />
    </Box>
  );
}

function run() {
  ReactDOM.render(<AdhocApp />, document.getElementById('root'));
}

// Since InlineChunkHtmlPlugin adds scripts to the head
// We must wait for the DOM to be loaded
// Otherwise React will fail to initialize since there's no DOM yet
window.addEventListener('DOMContentLoaded', run, false);
