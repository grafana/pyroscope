import React from 'react';
import ReactDOM from 'react-dom/client';
import '@phlare/jquery-import';
import { Provider } from 'react-redux';
import store from '@webapp/redux/store';
import '@webapp/../sass/profile.scss';
import '@szhsin/react-menu/dist/index.css';
import Notifications from '@webapp/ui/Notifications';
import { Router, Switch, Route } from 'react-router-dom';
import { createBrowserHistory } from 'history';

import { ROUTES } from '@phlare/pages/routes';
import { SingleView } from '@phlare/pages/SingleView';
import { ComparisonView } from '@phlare/pages/ComparisonView';
import { ExploreView } from '@phlare/pages/ExploreView';
import { DiffView } from '@phlare/pages/DiffView';
import { LoadAppNames } from '@phlare/components/LoadAppNames';
import { Sidebar } from '@phlare/components/Sidebar';
import { TenantWall } from '@phlare/components/TenantWall';
import { baseurl } from '@webapp/util/baseurl';

const container = document.getElementById('reactRoot') as HTMLElement;
const root = ReactDOM.createRoot(container);

function App() {
  const history = createBrowserHistory({ basename: baseurl() });

  return (
    <Router history={history}>
      <div className="app">
        <Sidebar />
        <div className="pyroscope-app">
          <TenantWall>
            <LoadAppNames>
              <Switch>
                <Route exact path={ROUTES.EXPLORE_VIEW}>
                  <ExploreView />
                </Route>
                <Route exact path={ROUTES.SINGLE_VIEW}>
                  <SingleView />
                </Route>
                <Route path={ROUTES.COMPARISON_VIEW}>
                  <ComparisonView />
                </Route>
                <Route path={ROUTES.COMPARISON_DIFF_VIEW}>
                  <DiffView />
                </Route>
              </Switch>
            </LoadAppNames>
          </TenantWall>
        </div>
      </div>
    </Router>
  );
}

root.render(
  <Provider store={store}>
    <Notifications />
    <App />
  </Provider>
);
