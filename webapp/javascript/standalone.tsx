import ReactDOM from 'react-dom';
import React from 'react';
import Box from '@webapp/ui/Box';
import { decodeFlamebearer } from '@webapp/models/flamebearer';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import '@pyroscope/flamegraph/dist/index.css';
import styles from './standalone.module.scss';

// Enable this if you are developing and don't want to run a server
// (window as any).flamegraph = defaultFlamegraph;

if (!(window as ShamefulAny).flamegraph) {
  alert(`'flamegraph' is required`);
  throw new Error(`'flamegraph' is required`);
}

// TODO parse window.flamegraph
const { flamegraph } = window as ShamefulAny;

// TODO: unify with the one in Footer component
function buildInfo() {
  const w = (window as ShamefulAny).buildInfo;
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

function StandaloneApp() {
  const flamebearer = decodeFlamebearer(flamegraph);

  return (
    <div>
      <Box className={styles.container}>
        <FlamegraphRenderer
          renderLogo
          flamebearer={flamebearer as any}
          ExportData={null}
        />
      </Box>
      <div className={styles.footer} title={buildInfo()}>
        {`Copyright © 2020 – ${new Date().getFullYear()} Pyroscope, Inc`}
      </div>
    </div>
  );
}

function run() {
  ReactDOM.render(<StandaloneApp />, document.getElementById('root'));
}

// Since InlineChunkHtmlPlugin adds scripts to the head
// We must wait for the DOM to be loaded
// Otherwise React will fail to initialize since there's no DOM yet
window.addEventListener('DOMContentLoaded', run, false);
