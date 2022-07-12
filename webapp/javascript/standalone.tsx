// @typescript-eslint/restrict-template-expressions
import ReactDOM from 'react-dom';
import React from 'react';
import Box from '@webapp/ui/Box';
import { decodeFlamebearer } from '@webapp/models/flamebearer';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import Footer from './components/Footer';
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

function StandaloneApp() {
  const flamebearer = decodeFlamebearer(flamegraph);

  return (
    <div>
      <Box className={styles.container}>
        <FlamegraphRenderer
          flamebearer={flamebearer as ShamefulAny}
          showCredit={false}
          ExportData={null}
        />
      </Box>
      <Footer />
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
