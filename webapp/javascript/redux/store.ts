import thunkMiddleware from 'redux-thunk';
import { createStore, applyMiddleware } from 'redux';
import { composeWithDevTools } from 'redux-devtools-extension';

import ReduxQuerySync from 'redux-query-sync';
import { configureStore } from '@reduxjs/toolkit';

import rootReducer from './reducers';
import history from '../util/history';

import viewsReducer from './reducers/views';
import {
  setLeftFrom,
  setLeftUntil,
  setRightFrom,
  setRightUntil,
  setFrom,
  setUntil,
  setMaxNodes,
  setQuery,
} from './actions';

import { parseLabels, encodeLabels } from '../util/key';

const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware)
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
);

const store = configureStore({
  reducer: {
    root: rootReducer,
    views: viewsReducer,
  },
  // middleware: [thunkMiddleware],
});

const defaultName = (window as any).initialState.appNames.find(
  (x) => x !== 'pyroscope.server.cpu'
);

ReduxQuerySync({
  store, // your Redux store
  params: {
    from: {
      defaultValue: 'now-1h',
      selector: (state) => state.root.from,
      action: setFrom,
    },
    until: {
      defaultValue: 'now',
      selector: (state) => state.root.until,
      action: setUntil,
    },
    leftFrom: {
      defaultValue: 'now-1h',
      selector: (state) => state.root.leftFrom,
      action: setLeftFrom,
    },
    leftUntil: {
      defaultValue: 'now-30m',
      selector: (state) => state.root.leftUntil,
      action: setLeftUntil,
    },
    rightFrom: {
      defaultValue: 'now-30m',
      selector: (state) => state.root.rightFrom,
      action: setRightFrom,
    },
    rightUntil: {
      defaultValue: 'now',
      selector: (state) => state.root.rightUntil,
      action: setRightUntil,
    },
    query: {
      defaultValue: `${defaultName || 'pyroscope.server.cpu'}{}`,
      selector: (state) => state.root.query,
      action: setQuery,
    },
    maxNodes: {
      defaultValue: '1024',
      selector: (state) => state.root.maxNodes,
      action: setMaxNodes,
    },
  },
  initialTruth: 'location',
  replaceState: false,
  history,
});
export default store;

// Infer the `RootState` and `AppDispatch` types from the store itself
export type RootState = ReturnType<typeof store.getState>;
// Inferred type: {posts: PostsState, comments: CommentsState, users: UsersState}
export type AppDispatch = typeof store.dispatch;
