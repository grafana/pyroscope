import React from 'react';
import ReactDOM from 'react-dom/client';
import './jquery-import';
import { Provider } from 'react-redux';
import store, { persistor } from './redux/store';
import '@webapp/../sass/profile.scss';
import '@szhsin/react-menu/dist/index.css';
import Notifications from '@webapp/ui/Notifications';

import { SingleView } from './pages/SingleView';

const container = document.getElementById('reactRoot') as HTMLElement;
const root = ReactDOM.createRoot(container);

root.render(
  <Provider store={store}>
    <Notifications />

    <SingleView />
  </Provider>
);
