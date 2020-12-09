import thunkMiddleware from 'redux-thunk';
import { createStore, compose, applyMiddleware } from "redux";
import persistState from 'redux-localstorage'
import updateUrl from "./enhancers/updateUrl";
import { composeWithDevTools } from 'redux-devtools-extension';

import ReduxQuerySync from 'redux-query-sync'

import rootReducer from "./reducers";
import {setFrom, setUntil, setLabels} from "./actions";

import {parseLabels, encodeLabels} from "../util/key.js";



const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware),
  // updateUrl(["from", "until", "labels"]),
  // persistState(["from", "until", "labels"]),
)

const store = createStore(rootReducer, enhancer);

ReduxQuerySync({
  store, // your Redux store
  params: {
    from: {
      defaultValue: "now-1h",
      selector: state => {
        return state.from;
      },
      action: setFrom,
    },
    until: {
      defaultValue: "now",
      selector: state => {
        return state.until;
      },
      action: setUntil,
    },
    name: {
      defaultValue: "unknown{}",
      selector: state => {
        return encodeLabels(state.labels);
      },
      action: (v) => {
        return setLabels(parseLabels(v));
      },
    },
  },
  initialTruth: 'location',
  replaceState: false,
})

export default store;
