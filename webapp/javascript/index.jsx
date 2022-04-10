import ReactDOM from 'react-dom';
import React, { useEffect } from 'react';

import { Provider, useDispatch } from 'react-redux';
import { Router, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import { isAdhocUIEnabled } from '@webapp/util/features';
import Notifications from '@webapp/ui/Notifications';
import { PersistGate } from 'redux-persist/integration/react';
import { loadCurrentUser } from '@webapp/redux/reducers/user';
import Footer from '@webapp/components/Footer';
import store, { persistor } from './redux/store';

import ContinuousSingleView from './pages/ContinuousSingleView';
import ContinuousComparisonView from './pages/ContinuousComparisonView';
import ContinuousDiffView from './pages/ContinuousDiffView';
import Continuous from './components/Continuous';
import Settings from './components/Settings';
import Sidebar from './components/Sidebar';
import AdhocSingle from './pages/AdhocSingle';
import AdhocComparison from './pages/AdhocComparison';
import AdhocDiff from './pages/AdhocDiff';
import ServiceDiscoveryApp from './components/ServiceDiscoveryApp';
import ServerNotifications from './components/ServerNotifications';
// since this style is practically all pages
import '@pyroscope/flamegraph/dist/index.css';

import SignInPage from './pages/IntroPages/SignIn';
import SignUpPage from './pages/IntroPages/SignUp';
import NotFound from './pages/IntroPages/NotFound';

import history from './util/history';

let showFps = false;
try {
  // run this to enable FPS meter:
  //  window.localStorage.setItem('showFps', true);
  showFps = window.localStorage.getItem('showFps');
} catch (e) {
  console.error(e);
}

function App() {
  return (
    <div className="app">
      <Sidebar />
      <div className="pyroscope-app">
        <Switch>
          <Route exact path="/login">
            <SignInPage />
          </Route>
          <Route exact path="/signup">
            <SignUpPage />
          </Route>
          <Route exact path="/">
            <Continuous>
              <ContinuousSingleView />
            </Continuous>
          </Route>
          <Route path="/comparison">
            <Continuous>
              <ContinuousComparisonView />
            </Continuous>
          </Route>
          <Route path="/comparison-diff">
            <Continuous>
              <ContinuousDiffView />
            </Continuous>
          </Route>
          <Route path="/settings">
            <Settings />
          </Route>
          <Route path="/service-discovery">
            <ServiceDiscoveryApp />
          </Route>
          {isAdhocUIEnabled && [
            <Route path="/adhoc-single">
              <AdhocSingle />
            </Route>,
            <Route path="/adhoc-comparison">
              <AdhocComparison />
            </Route>,
            <Route path="/adhoc-comparison-diff">
              <AdhocDiff />
            </Route>,
          ]}
          <Route path="*" exact>
            <NotFound />
          </Route>
        </Switch>
        <Footer />
      </div>
    </div>
  );
}

ReactDOM.render(
  <Provider store={store}>
    <PersistGate persistor={persistor} loading={null}>
      <Router history={history}>
        <ServerNotifications />
        <Notifications />
        <App />
      </Router>
      {showFps ? <FPSStats left="auto" top="auto" bottom={2} right={2} /> : ''}
    </PersistGate>
  </Provider>,
  document.getElementById('root')
);
