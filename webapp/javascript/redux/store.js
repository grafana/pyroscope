import thunkMiddleware from "redux-thunk";
import promiseMiddleware from "redux-promise";

import { createStore, applyMiddleware } from "redux";
import { composeWithDevTools } from "redux-devtools-extension";

import ReduxQuerySync from "redux-query-sync";

import rootReducer from "./reducers";
import history from "../util/history";

import {
  setLeftFrom,
  setLeftUntil,
  setRightFrom,
  setRightUntil,
  setFrom,
  setUntil,
  setMaxNodes,
  setQuery,
} from "./actions";

import { parseLabels, encodeLabels } from "../util/key";

const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware, promiseMiddleware)
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
);

const store = createStore(rootReducer, enhancer);

const defaultName = window.initialState.appNames.find(
  (x) => x !== "pyroscope.server.cpu"
);

ReduxQuerySync({
  store, // your Redux store
  params: {
    from: {
      defaultValue: "now-1h",
      selector: (state) => state.from,
      action: setFrom,
    },
    until: {
      defaultValue: "now",
      selector: (state) => state.until,
      action: setUntil,
    },
    leftFrom: {
      defaultValue: "now-1h",
      selector: (state) => state.leftFrom,
      action: setLeftFrom,
    },
    leftUntil: {
      defaultValue: "now-30m",
      selector: (state) => state.leftUntil,
      action: setLeftUntil,
    },
    rightFrom: {
      defaultValue: "now-30m",
      selector: (state) => state.rightFrom,
      action: setRightFrom,
    },
    rightUntil: {
      defaultValue: "now",
      selector: (state) => state.rightUntil,
      action: setRightUntil,
    },
    query: {
      defaultValue: `${defaultName || "pyroscope.server.cpu"}{}`,
      selector: (state) => state.query,
      action: setQuery,
    },
    maxNodes: {
      defaultValue: "1024",
      selector: (state) => state.maxNodes,
      action: setMaxNodes,
    },
  },
  initialTruth: "location",
  replaceState: false,
  history,
});

export default store;
