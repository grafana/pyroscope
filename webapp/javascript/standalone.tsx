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

// TODO: unify with the one in Footer component
function buildInfo() {
  const w = (window as any).buildInfo;
  return `
    BUILD INFO:
    goos: ${w.goos}
    goarch: ${w.goarch}
    go_version: ${w.goVersion}
    version: ${w.version}
    time: ${w.time}
    gitSHA: ${w.gitSHA}
    gitDirty: ${w.gitDirty}
    embeddedAssets: ${w.useEmbeddedAssets}
`.replace(/^\s+/gm, '');
}

function AdhocApp() {
  const flamebearer = decodeFlamebearer(flamegraph);

  const viewType =
    flamebearer.format === 'single' ? flamebearer.format : 'diff';

  return (
    <div>
      <Box className={styles.container}>
        <FlameGraphRenderer
          renderLogo
          flamebearer={flamebearer}
          viewType={viewType}
          display="both"
          rawFlamegraph={flamegraph}
          ExportData={null}
        />
      </Box>
      <div
        style={{ textAlign: 'center', padding: '30px 0' }}
        title={buildInfo()}
      >
        {`Copyright © 2020 – ${new Date().getFullYear()} Pyroscope, Inc`}
      </div>
    </div>
  );
}

function run() {
  ReactDOM.render(<AdhocApp />, document.getElementById('root'));
}

// Since InlineChunkHtmlPlugin adds scripts to the head
// We must wait for the DOM to be loaded
// Otherwise React will fail to initialize since there's no DOM yet
window.addEventListener('DOMContentLoaded', run, false);
