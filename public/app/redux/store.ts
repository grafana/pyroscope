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

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: Until we rewrite FlamegraphRenderer in typescript this will do
import { configureStore, combineReducers, Middleware } from '@reduxjs/toolkit';

import tracingReducer from '@pyroscope/redux/reducers/tracing';

import settingsReducer from './reducers/settings';
import userReducer from './reducers/user';
import { continuousReducer } from './reducers/continuous';
import serviceDiscoveryReducer from './reducers/serviceDiscovery';
import adhocReducer from '@pyroscope/redux/reducers/adhoc';
import uiStore, { persistConfig as uiPersistConfig } from './reducers/ui';
import tenantReducer, {
  persistConfig as tenantPersistConfig,
} from '@pyroscope/redux/reducers/tenant';
import { setStore } from '@pyroscope/services/storage';

const reducer = combineReducers({
  settings: settingsReducer,
  user: userReducer,
  serviceDiscovery: serviceDiscoveryReducer,
  ui: persistReducer(uiPersistConfig, uiStore),
  continuous: continuousReducer,
  tenant: persistReducer(tenantPersistConfig, tenantReducer),
  tracing: tracingReducer,
  adhoc: adhocReducer,
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
  // https://github.com/reduxjs/redux-toolkit/issues/587#issuecomment-824927971
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware({
      serializableCheck: {
        ignoredActionPaths: ['error'],

        // Based on this issue: https://github.com/rt2zz/redux-persist/issues/988
        // and this guide https://redux-toolkit.js.org/usage/usage-guide#use-with-redux-persist
        ignoredActions: [
          FLUSH,
          REHYDRATE,
          PAUSE,
          PERSIST,
          PURGE,
          REGISTER,
          'adhoc/uploadFile/pending',
          'adhoc/uploadFile/fulfilled',
        ],
      },
    }).concat([logErrorMiddleware]),
});

export const persistor = persistStore(store);

export default store;

// Infer the `RootState` and `AppDispatch` types from the store itself
export type RootState = ReturnType<typeof store.getState>;
// Inferred type: {posts: PostsState, comments: CommentsState, users: UsersState}
export type AppDispatch = typeof store.dispatch;

export type StoreType = typeof store;

setStore(store);
