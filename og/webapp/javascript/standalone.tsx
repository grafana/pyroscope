// @typescript-eslint/restrict-template-expressions
import ReactDOM from 'react-dom';
import React from 'react';
import Box from '@webapp/ui/Box';
import { decodeFlamebearer } from '@webapp/models/flamebearer';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import Footer from './components/Footer';
import '@pyroscope/flamegraph/dist/index.css';

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
      <Box>
        <FlamegraphRenderer
          flamebearer={flamebearer as ShamefulAny}
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
