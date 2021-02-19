import thunkMiddleware from "redux-thunk";
import promiseMiddleware from "redux-promise";

import { createStore, applyMiddleware } from "redux";
import { composeWithDevTools } from "redux-devtools-extension";

import ReduxQuerySync from "redux-query-sync";

import rootReducer from "./reducers";
import history from "../util/history";

import { setFrom, setUntil, setLabels, setMaxNodes } from "./actions";

import { parseLabels, encodeLabels } from "../util/key";

const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware, promiseMiddleware)
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
);

const store = createStore(rootReducer, enhancer);

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
    name: {
      selector: (state) => encodeLabels(state.labels),
      action: (v) => {
        const labels = parseLabels(v);
        if (labels.length > 0) {
          return setLabels(labels);
        }
        return { type: "NOOP" };
      },
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
