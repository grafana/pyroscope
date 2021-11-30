import ReactDOM from 'react-dom';
import React from 'react';

import { Provider } from 'react-redux';
import { ShortcutProvider } from 'react-keybind';
import { Router, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import store from './redux/store';

import PyroscopeApp from './components/PyroscopeApp';
import ComparisonApp from './components/ComparisonApp';
import ComparisonDiffApp from './components/ComparisonDiffApp';
import Sidebar from './components/Sidebar';
import Notifications from './components/Notifications';
import AdhocSingle from './components/AdhocSingle';

import history from './util/history';

import '../../wasm-poc/wasm_exec';
import wasmData from '../../wasm-poc/main.wasm';

function base64ToArrayBuffer(base64) {
  const binaryStr = window.atob(base64);
  const len = binaryStr.length;
  const bytes = new Uint8Array(len);
  for (let i = 0; i < len; i += 1) {
    bytes[i] = binaryStr.charCodeAt(i);
  }
  return bytes.buffer;
}

const go = new Go();
// TODO: it would be nice to get an array buffer straight from webpack
WebAssembly.instantiate(
  base64ToArrayBuffer(wasmData.split(',', 2)[1]),
  go.importObject
).then((result) => {
  go.run(result.instance);
  console.log(wasm);
  wasm.print('Hello from go!');

  let showFps = false;
  try {
    // run this to enable FPS meter:
    //   window.localStorage.setItem("showFps", true);
    showFps = window.localStorage.getItem('showFps');
  } catch (e) {
    console.error(e);
  }

  // TODO fetch this from localstorage?
  const enableAdhoc = true;

  ReactDOM.render(
    <Provider store={store}>
      <Router history={history}>
        <ShortcutProvider>
          <Sidebar />
          <Switch>
            <Route exact path="/">
              <PyroscopeApp />
            </Route>
            <Route path="/comparison">
              <ComparisonApp />
            </Route>
            <Route path="/comparison-diff">
              <ComparisonDiffApp />
            </Route>
            {enableAdhoc && (
              <Route path="/adhoc-single">
                <AdhocSingle />
              </Route>
            )}
          </Switch>
          <Notifications />
        </ShortcutProvider>
      </Router>
      {showFps ? <FPSStats left="auto" top="auto" bottom={2} right={2} /> : ''}
    </Provider>,
    document.getElementById('root')
  );
});
