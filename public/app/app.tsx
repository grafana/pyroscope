import React from 'react';
import ReactDOM from 'react-dom/client';
import '@pyroscope/jquery-import';
import { Provider } from 'react-redux';
import store from '@pyroscope/redux/store';
import './sass/profile.scss';
import '@szhsin/react-menu/dist/index.css';
import Notifications from '@pyroscope/ui/Notifications';
import { Router, Switch, Route } from 'react-router-dom';
import { ROUTES } from '@pyroscope/pages/routes';
import { SingleView } from '@pyroscope/pages/SingleView';
import { ComparisonView } from '@pyroscope/pages/ComparisonView';
import { ExploreView } from '@pyroscope/pages/ExploreView';
import { DiffView } from '@pyroscope/pages/DiffView';
import { Sidebar } from '@pyroscope/components/Sidebar';
import { TenantWall } from '@pyroscope/components/TenantWall';
import { history } from '@pyroscope/util/history';
import { useSelectFirstApp } from '@pyroscope/hooks/useAppNames';

const container = document.getElementById('reactRoot') as HTMLElement;
const root = ReactDOM.createRoot(container);

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
