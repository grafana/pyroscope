import './globals';

import ReactDOM from 'react-dom';
import React from 'react';
import { Provider } from 'react-redux';
import { Router, Switch, Route } from 'react-router-dom';
import { isAdhocUIEnabled, isAuthRequired } from '@webapp/util/features';
import Notifications from '@webapp/ui/Notifications';
import { PersistGate } from 'redux-persist/integration/react';
import Footer from '@webapp/components/Footer';
import PageTitle from '@webapp/components/PageTitle';
import store, { persistor } from './redux/store';

import ContinuousSingleView from './pages/ContinuousSingleView';
import ContinuousComparisonView from './pages/ContinuousComparisonView';
import ContinuousDiffView from './pages/ContinuousDiffView';
import TagExplorerView from './pages/TagExplorerView';
import Continuous from './components/Continuous';
import Settings from './components/Settings';
import Sidebar from './components/Sidebar';
import AdhocSingle from './pages/adhoc/AdhocSingle';
import AdhocComparison from './pages/adhoc/AdhocComparison';
import AdhocDiff from './pages/adhoc/AdhocDiff';
import ServiceDiscoveryApp from './pages/ServiceDiscovery';
import ServerNotifications from './components/ServerNotifications';
import Protected from './components/Protected';
import SignInPage from './pages/IntroPages/SignIn';
import SignUpPage from './pages/IntroPages/SignUp';
import Forbidden from './pages/IntroPages/Forbidden';
import NotFound from './pages/IntroPages/NotFound';
import { PAGES } from './pages/constants';
import history from './util/history';
import TracingSingleView from './pages/TracingSingleView';
import ExemplarsSingleView from './pages/exemplars/ExemplarsSingleView';

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
          <Route path={PAGES.TRACING_EXEMPLARS_MERGE}>
            <Protected>
              <Continuous>
                <TracingSingleView />
              </Continuous>
            </Protected>
          </Route>
          <Route path={PAGES.TRACING_EXEMPLARS_SINGLE}>
            <Protected>
              <Continuous>
                <ExemplarsSingleView />
              </Continuous>
            </Protected>
          </Route>
          {isAuthRequired && (
            <Route path={PAGES.SETTINGS}>
              <Protected>
                <Continuous>
                  <Settings />
                </Continuous>
              </Protected>
            </Route>
          )}
          <Route path={PAGES.SERVICE_DISCOVERY}>
            <Protected>
              <PageTitle title="Pull Targets" />
              <ServiceDiscoveryApp />
            </Protected>
          </Route>
          <Route exact path={PAGES.TAG_EXPLORER}>
            <Protected>
              <Continuous>
                <TagExplorerView />
              </Continuous>
            </Protected>
          </Route>
          {isAdhocUIEnabled && (
            <Route path={PAGES.ADHOC_SINGLE}>
              <Protected>
                <PageTitle title="Adhoc Single" />
                <AdhocSingle />
              </Protected>
            </Route>
          )}
          {isAdhocUIEnabled && (
            <Route path={PAGES.ADHOC_COMPARISON}>
              <Protected>
                <PageTitle title="Adhoc Comparison" />
                <AdhocComparison />
              </Protected>
            </Route>
          )}
          {isAdhocUIEnabled && (
            <Route path={PAGES.ADHOC_COMPARISON_DIFF}>
              <Protected>
                <PageTitle title="Adhoc Diff" />
                <AdhocDiff />
              </Protected>
            </Route>
          )}
          <Route path={PAGES.FORBIDDEN}>
            <>
              <PageTitle title="Forbidden" />
              <Forbidden />
            </>
          </Route>

          <Route path="*" exact>
            <>
              <PageTitle title="Not Found" />
              <NotFound />
            </>
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
    </PersistGate>
  </Provider>,
  document.getElementById('root')
);
