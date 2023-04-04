import React from 'react';
import ReactDOM from 'react-dom/client';
import './jquery-import';
import { Provider } from 'react-redux';
import store, { persistor } from './redux/store';
import '@webapp/../sass/profile.scss';
import '@szhsin/react-menu/dist/index.css';

import { SingleView } from './pages/SingleView';

const root = ReactDOM.createRoot(document.getElementById('reactRoot'));
root.render(
  <Provider store={store}>
    <SingleView />
  </Provider>
);
