import React from 'react';
import { Sidebar } from '@pyroscope/components/Sidebar';
import { TenantWall } from '@pyroscope/components/TenantWall';
import { useSelectFirstApp } from '@pyroscope/hooks/useAppNames';
import '@pyroscope/jquery-import';
import { ComparisonView } from '@pyroscope/pages/ComparisonView';
import { DiffView } from '@pyroscope/pages/DiffView';
import { ExploreView } from '@pyroscope/pages/ExploreView';
import { SingleView } from '@pyroscope/pages/SingleView';
import { ROUTES } from '@pyroscope/pages/routes';
import store from '@pyroscope/redux/store';
import Notifications from '@pyroscope/ui/Notifications';
import { history } from '@pyroscope/util/history';
import '@szhsin/react-menu/dist/index.css';
import ReactDOM from 'react-dom/client';
import { Provider } from 'react-redux';
import { Route, Router, Switch } from 'react-router-dom';
import { setupReduxQuerySync } from './redux/useReduxQuerySync';
import './sass/profile.scss';

const container = document.getElementById('reactRoot') as HTMLElement;
const root = ReactDOM.createRoot(container);

setupReduxQuerySync();

declare global {
  interface Window {
    __grafana_public_path__: string;
  }
}

if (typeof window !== 'undefined') {
  // Icons from @grafana/ui are not bundled, this forces them to be loaded via a CDN instead.
  window.__grafana_public_path__ = 'assets/grafana/';
}

function App() {
  useSelectFirstApp();

  return (
    <Router history={history}>
      <div className="app">
        <Sidebar />
        <div className="pyroscope-app">
          <TenantWall>
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
