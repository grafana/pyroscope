import React from 'react';
import ReactDOM from 'react-dom/client';
import './jquery-import';
import { Provider } from 'react-redux';
import store, { persistor } from './redux/store';
import '@webapp/../sass/profile.scss';
import '@szhsin/react-menu/dist/index.css';
import Notifications from '@webapp/ui/Notifications';
import { Router, Switch, Route } from 'react-router-dom';
import { createBrowserHistory } from 'history';

import { SingleView } from './pages/SingleView';
import { ComparisonView } from './pages/ComparisonView';
import { LoadAppNames } from './components/LoadAppNames';

const container = document.getElementById('reactRoot') as HTMLElement;
const root = ReactDOM.createRoot(container);

function App() {
  // Defined statically in webpack when building
  const basepath = process.env.BASEPATH ? process.env.BASEPATH : '';
  const history = createBrowserHistory({ basename: basepath });

  return (
    <Provider store={store}>
      <Notifications />
      <LoadAppNames>
        <Router history={history}>
          <Switch>
            <Route exact path={'/'}>
              <SingleView />
            </Route>
            <Route path={'/comparison'}>
              <ComparisonView />
            </Route>
          </Switch>
        </Router>
      </LoadAppNames>
    </Provider>
  );
}

root.render(<App />);
