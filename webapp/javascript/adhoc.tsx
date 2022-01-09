import ReactDOM from 'react-dom';
import React from 'react';

function run() {
  ReactDOM.render(<div>Hello world</div>, document.getElementById('root'));
}

// Since InlineChunkHtmlPlugin adds scripts to the head
// We must wait for the DOM to be loaded
// Otherwise React will fail to initialize since there's no DOM yet
window.addEventListener('DOMContentLoaded', run, false);
