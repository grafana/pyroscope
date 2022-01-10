import thunkMiddleware from 'redux-thunk';
import { createStore, applyMiddleware } from 'redux';
import { composeWithDevTools } from 'redux-devtools-extension';

import ReduxQuerySync from 'redux-query-sync';
import { configureStore, getDefaultMiddleware } from '@reduxjs/toolkit';

import createSagaMiddleware from 'redux-saga';
import createSagaMonitor from '@clarketm/saga-monitor';

import { mainSaga } from './saga';

import rootReducer from './reducers';
import history from '../util/history';

import viewsReducer from './reducers/views';
import newRootStore from './reducers/newRoot';

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

const devMode = process.env.NODE_ENV === 'development';

const sagaMonitorConfig = {
  level: 'log',
  effectTrigger: true,
  effectResolve: true,
  actionDispatch: false,
};

const sagaMiddleware = createSagaMiddleware({
  sagaMonitor: devMode ? createSagaMonitor(sagaMonitorConfig) : undefined,
});

const enhancer = composeWithDevTools(
  applyMiddleware(sagaMiddleware)
  // applyMiddleware(thunkMiddleware)
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
);

const middleware = [...getDefaultMiddleware(), sagaMiddleware];

const store = configureStore({
  reducer: {
    newRoot: newRootStore,
    root: rootReducer,
    views: viewsReducer,
  },
  middleware,
  // middleware: [thunkMiddleware],
});
sagaMiddleware.run(mainSaga);

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
  replaceState: true,
  history,
});

export default store;

// Infer the `RootState` and `AppDispatch` types from the store itself
export type RootState = ReturnType<typeof store.getState>;
// Inferred type: {posts: PostsState, comments: CommentsState, users: UsersState}
export type AppDispatch = typeof store.dispatch;
