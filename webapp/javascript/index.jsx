import ReactDOM from 'react-dom';
import React from 'react';

import { Provider } from 'react-redux';
import { ShortcutProvider } from 'react-keybind';
import { Router, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import Notifications from '@ui/Notifications';
import store from './redux/store';

import PyroscopeApp from './components/PyroscopeApp';
import ComparisonApp from './components/ComparisonApp';
import ComparisonDiffApp from './components/ComparisonDiffApp';
import Sidebar from './components/Sidebar';
import AdhocSingle from './components/AdhocSingle';

import history from './util/history';

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
        <Notifications />
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
      </ShortcutProvider>
    </Router>
    {showFps ? <FPSStats left="auto" top="auto" bottom={2} right={2} /> : ''}
  </Provider>,
  document.getElementById('root')
);
