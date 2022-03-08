import {
  persistStore,
  persistReducer,
  FLUSH,
  REHYDRATE,
  PAUSE,
  PERSIST,
  PURGE,
  REGISTER,
  createMigrate,
} from 'redux-persist';
import thunkMiddleware from 'redux-thunk';
import { createStore, applyMiddleware } from 'redux';
import { composeWithDevTools } from 'redux-devtools-extension';

import ReduxQuerySync from 'redux-query-sync';
import { configureStore, combineReducers } from '@reduxjs/toolkit';
import storage from 'redux-persist/lib/storage';

import rootReducer from './reducers';
import history from '../util/history';

import viewsReducer from './reducers/views';
import newRootStore from './reducers/newRoot';
import settingsReducer from './reducers/settings';
import userReducer from './reducers/user';
import continuousReducer from './reducers/continuous';
import serviceDiscoveryReducer from './reducers/serviceDiscovery';
import uiStore, { persistConfig as uiPersistConfig } from './reducers/ui';

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

const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware)
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
);

const reducer = combineReducers({
  newRoot: newRootStore,
  root: rootReducer,
  views: viewsReducer,
  settings: settingsReducer,
  user: userReducer,
  serviceDiscovery: serviceDiscoveryReducer,
  ui: persistReducer(uiPersistConfig, uiStore),
  continuous: continuousReducer,
});

const store = configureStore({
  reducer,
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware({
      serializableCheck: {
        // Based on this issue: https://github.com/rt2zz/redux-persist/issues/988
        // and this guide https://redux-toolkit.js.org/usage/usage-guide#use-with-redux-persist
        ignoredActions: [FLUSH, REHYDRATE, PAUSE, PERSIST, PURGE, REGISTER],
      },
    }),
});

export const persistor = persistStore(store);

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
      defaultvalue: '',
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
