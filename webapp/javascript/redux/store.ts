import thunkMiddleware from 'redux-thunk';
import { applyMiddleware } from 'redux';
import { composeWithDevTools } from 'redux-devtools-extension';

import ReduxQuerySync from 'redux-query-sync';
import { configureStore, combineReducers } from '@reduxjs/toolkit';

import { persistStore, persistReducer } from 'redux-persist';
import storage from 'redux-persist/lib/storage';
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

import { parseLabels, encodeLabels } from '../util/key'; // defaults to localStorage for web

const persistConfig = {
  key: 'pyroscope',
  storage,
};

const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware)
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
);
const reducer = persistReducer(
  persistConfig,
  combineReducers({
    root: rootReducer,
    views: viewsReducer,
  })
);
const store = configureStore({
  reducer,
  // middleware: [thunkMiddleware],
});

export const persistor = persistStore(store);

const defaultName = window.initialState.appNames.find(
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
