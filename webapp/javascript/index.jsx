import ReactDOM from 'react-dom';
import React from 'react';

import { Provider } from 'react-redux';
import { BrowserRouter, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import { isExperimentalAdhocUIEnabled } from '@utils/features';
import Notifications from '@ui/Notifications';
import store from './redux/store';

import PyroscopeApp from './components/PyroscopeApp';
import ComparisonApp from './components/ComparisonApp';
import ComparisonDiffApp from './components/ComparisonDiffApp';
import Sidebar from './components/Sidebar';
import AdhocSingle from './components/AdhocSingle';
import AdhocComparison from './components/AdhocComparison';
import ServerNotifications from './components/ServerNotifications';

import history from './util/history';
import basename from './util/baseurl';

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
    <BrowserRouter history={history} basename={basename()}>
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
          {isExperimentalAdhocUIEnabled && (
            <Route path="/adhoc-single">
              <AdhocSingle />
            </Route>
          )}
          {isExperimentalAdhocUIEnabled && (
            <Route path="/adhoc-comparison">
              <AdhocComparison />
            </Route>
          )}
        </Switch>
      </div>
    </BrowserRouter>
    {showFps ? <FPSStats left="auto" top="auto" bottom={2} right={2} /> : ''}
  </Provider>,
  document.getElementById('root')
);
