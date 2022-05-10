import ReactDOM from 'react-dom';
import React from 'react';

import { Provider } from 'react-redux';
import { Router, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import { isAdhocUIEnabled } from '@webapp/util/features';
import Notifications from '@webapp/ui/Notifications';
import { PersistGate } from 'redux-persist/integration/react';
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
import ServiceDiscoveryApp from './pages/ServiceDiscovery';
import ServerNotifications from './components/ServerNotifications';
import Protected from './components/Protected';
// since this style is practically all pages
import '@pyroscope/flamegraph/dist/index.css';

import SignInPage from './pages/IntroPages/SignIn';
import SignUpPage from './pages/IntroPages/SignUp';
import Forbidden from './pages/IntroPages/Forbidden';
import NotFound from './pages/IntroPages/NotFound';
import { PAGES } from './pages/constants';
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
          <Route exact path={PAGES.LOGIN}>
            <SignInPage />
          </Route>
          <Route exact path={PAGES.SIGNUP}>
            <SignUpPage />
          </Route>
          <Route exact path={PAGES.CONTINOUS_SINGLE_VIEW}>
            <Protected>
              <Continuous>
                <ContinuousSingleView />
              </Continuous>
            </Protected>
          </Route>
          <Route path={PAGES.COMPARISON_VIEW}>
            <Protected>
              <Continuous>
                <ContinuousComparisonView />
              </Continuous>
            </Protected>
          </Route>
          <Route path={PAGES.COMPARISON_DIFF_VIEW}>
            <Protected>
              <Continuous>
                <ContinuousDiffView />
              </Continuous>
            </Protected>
          </Route>
          <Route path={PAGES.SETTINGS}>
            <Protected>
              <Continuous>
                <Settings />
              </Continuous>
            </Protected>
          </Route>
          <Route path={PAGES.SERVICE_DISCOVERY}>
            <Protected>
              <ServiceDiscoveryApp />
            </Protected>
          </Route>
          {isAdhocUIEnabled && (
            <Route path={PAGES.ADHOC_SINGLE}>
              <Protected>
                <AdhocSingle />
              </Protected>
            </Route>
          )}
          {isAdhocUIEnabled && (
            <Route path={PAGES.ADHOC_COMPARISON}>
              <Protected>
                <AdhocComparison />
              </Protected>
            </Route>
          )}
          {isAdhocUIEnabled && (
            <Route path={PAGES.ADHOC_COMPARISON_DIFF}>
              <Protected>
                <AdhocDiff />
              </Protected>
            </Route>
          )}
          <Route path={PAGES.FORBIDDEN}>
            <Forbidden />
          </Route>

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
