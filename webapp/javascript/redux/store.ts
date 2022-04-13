import {
  persistStore,
  persistReducer,
  FLUSH,
  REHYDRATE,
  PAUSE,
  PERSIST,
  PURGE,
  REGISTER,
} from 'redux-persist';
import { deserializeError } from 'serialize-error';

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: Until we rewrite FlamegraphRenderer in typescript this will do
import ReduxQuerySync from 'redux-query-sync';
import { configureStore, combineReducers, Middleware } from '@reduxjs/toolkit';

import rootReducer from './reducers';
import history from '../util/history';

import viewsReducer from './reducers/views';
import settingsReducer from './reducers/settings';
import userReducer from './reducers/user';
import continuousReducer, {
  actions as continuousActions,
} from './reducers/continuous';
import serviceDiscoveryReducer from './reducers/serviceDiscovery';
import uiStore, { persistConfig as uiPersistConfig } from './reducers/ui';

const reducer = combineReducers({
  root: rootReducer,
  views: viewsReducer,
  settings: settingsReducer,
  user: userReducer,
  serviceDiscovery: serviceDiscoveryReducer,
  ui: persistReducer(uiPersistConfig, uiStore),
  continuous: continuousReducer,
});

// Most times we will display a (somewhat) user friendly message toast
// But it's still useful to have the actual error logged to the console
export const logErrorMiddleware: Middleware = () => (next) => (action) => {
  next(action);
  if (action?.error) {
    console.error(action.error);
  }
};

const store = configureStore({
  reducer,
  middleware: (getDefaultMiddleware) => [
    ...getDefaultMiddleware({
      serializableCheck: {
        ignoredActionPaths: ['error'],

        // Based on this issue: https://github.com/rt2zz/redux-persist/issues/988
        // and this guide https://redux-toolkit.js.org/usage/usage-guide#use-with-redux-persist
        ignoredActions: [FLUSH, REHYDRATE, PAUSE, PERSIST, PURGE, REGISTER],
      },
    }),

    logErrorMiddleware,
  ],
});

export const persistor = persistStore(store);

// This is a bi-directional sync between the query parameters and the redux store
// It works as follows:
// * When URL query changes, It will dispatch the action
// * When the store changes (the field set in selector), the query param is updated
// For more info see the implementation at
// https://github.com/Treora/redux-query-sync/blob/master/src/redux-query-sync.js
ReduxQuerySync({
  store,
  params: {
    from: {
      defaultValue: 'now-1h',
      selector: (state: RootState) => state.continuous.from,
      action: continuousActions.setFrom,
    },
    until: {
      defaultValue: 'now',
      selector: (state: RootState) => state.continuous.until,
      action: continuousActions.setUntil,
    },
    leftFrom: {
      defaultValue: 'now-1h',
      selector: (state: RootState) => state.continuous.leftFrom,
      action: continuousActions.setLeftFrom,
    },
    leftUntil: {
      defaultValue: 'now-30m',
      selector: (state: RootState) => state.continuous.leftUntil,
      action: continuousActions.setLeftUntil,
    },
    rightFrom: {
      defaultValue: 'now-30m',
      selector: (state: RootState) => state.continuous.rightFrom,
      action: continuousActions.setRightFrom,
    },
    rightUntil: {
      defaultValue: 'now',
      selector: (state: RootState) => state.continuous.rightUntil,
      action: continuousActions.setRightUntil,
    },
    query: {
      defaultvalue: '',
      selector: (state: RootState) => state.continuous.query,
      action: continuousActions.setQuery,
    },
    rightQuery: {
      defaultvalue: '',
      selector: (state: RootState) => state.continuous.rightQuery,
      action: continuousActions.setRightQuery,
    },
    leftQuery: {
      defaultvalue: '',
      selector: (state: RootState) => state.continuous.leftQuery,
      action: continuousActions.setLeftQuery,
    },
    maxNodes: {
      defaultValue: '1024',
      selector: (state: RootState) => state.continuous.maxNodes,
      action: continuousActions.setMaxNodes,
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
