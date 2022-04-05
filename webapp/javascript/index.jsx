import ReactDOM from 'react-dom';
import React, { useEffect } from 'react';

import { Provider, useDispatch } from 'react-redux';
import { Router, Switch, Route } from 'react-router-dom';
import FPSStats from 'react-fps-stats';
import { isAdhocUIEnabled } from '@webapp/util/features';
import Notifications from '@webapp/ui/Notifications';
import { PersistGate } from 'redux-persist/integration/react';
import { loadCurrentUser } from '@webapp/redux/reducers/user';
import store, { persistor } from './redux/store';

import ContinuousSingleView from './pages/ContinuousSingleView';
import ContinuousComparisonView from './pages/ContinuousComparisonView';
import ContinuousDiffView from './pages/ContinuousDiffView';
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

const createRoutes = (isAdhocUIEnabled) => {
  if (isAdhocUIEnabled) {
    return (
      <Switch>
        <Route exact path="/login">
          <SignInPage />
        </Route>
        <Route exact path="/signup">
          <SignUpPage />
        </Route>
        <Route exact path="/">
          <Sidebar />
          <ContinuousSingleView />
        </Route>
        <Route exact path="/comparison">
          <Sidebar />
          <ContinuousComparisonView />
        </Route>
        <Route exact path="/comparison-diff">
          <Sidebar />
          <ContinuousDiffView />
        </Route>
        <Route exact path="/settings">
          <Sidebar />
          <Settings />
        </Route>
        <Route exact path="/service-discovery">
          <Sidebar />
          <ServiceDiscoveryApp />
        </Route>
        <Route path="/adhoc-single">
          <AdhocSingle />
        </Route>
        <Route path="/adhoc-comparison">
          <AdhocComparison />
        </Route>
        <Route path="/adhoc-comparison-diff">
          <AdhocDiff />
        </Route>
        <Route path="*" exact>
          <NotFound />
        </Route>
      </Switch>
    );
  }

  return (
    <Switch>
      <Route exact path="/login">
        <SignInPage />
      </Route>
      <Route exact path="/signup">
        <SignUpPage />
      </Route>
      <Route exact path="/">
        <Sidebar />
        <ContinuousSingleView />
      </Route>
      <Route exact path="/comparison">
        <Sidebar />
        <ContinuousComparisonView />
      </Route>
      <Route exact path="/comparison-diff">
        <Sidebar />
        <ContinuousDiffView />
      </Route>
      <Route exact path="/settings">
        <Sidebar />
        <Settings />
      </Route>
      <Route exact path="/service-discovery">
        <Sidebar />
        <ServiceDiscoveryApp />
      </Route>

      <Route path="*" exact>
        <NotFound />
      </Route>
    </Switch>
  );
};

function App() {
  const dispatch = useDispatch();

  useEffect(() => {
    if (window.isAuthRequired) {
      dispatch(loadCurrentUser());
    }
  }, [dispatch]);

  return <div className="app">{createRoutes()}</div>;
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
