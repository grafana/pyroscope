import ReactDOM from 'react-dom';
import React from 'react';

import { Provider } from 'react-redux';
import { Router, BrowserRouter, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import { isAdhocUIEnabled } from '@utils/features';
import Notifications from '@ui/Notifications';
import { PersistGate } from 'redux-persist/integration/react';
import store, { persistor } from './redux/store';

import PyroscopeApp from './components/PyroscopeApp';
import ComparisonApp from './components/ComparisonApp';
import ComparisonDiffApp from './components/ComparisonDiffApp';
import Sidebar from './components/Sidebar';
import AdhocSingle from './components/AdhocSingle';
import AdhocComparison from './components/AdhocComparison';
import AdhocComparisonDiff from './components/AdhocComparisonDiff';
import ServerNotifications from './components/ServerNotifications';

import history from './util/history';

let showFps = false;
try {
  // run this to enable FPS meter:
  //  window.localStorage.setItem('showFps', true);
  showFps = window.localStorage.getItem('showFps');
} catch (e) {
  console.error(e);
}

ReactDOM.render(
  <Provider store={store}>
    <PersistGate persistor={persistor} loading={null}>
      <Router history={history}>
        <ServerNotifications />
        <Notifications />
        <div className="app">
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
            {isAdhocUIEnabled && (
              <Route path="/adhoc-single">
                <AdhocSingle />
              </Route>
            )}
            {isAdhocUIEnabled && (
              <Route path="/adhoc-comparison">
                <AdhocComparison />
              </Route>
            )}
            {isAdhocUIEnabled && (
              <Route path="/adhoc-comparison-diff">
                <AdhocComparisonDiff />
              </Route>
            )}
          </Switch>
        </div>
      </Router>
      {showFps ? <FPSStats left="auto" top="auto" bottom={2} right={2} /> : ''}
    </PersistGate>
  </Provider>,
  document.getElementById('root')
);
