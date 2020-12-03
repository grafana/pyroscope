import thunkMiddleware from 'redux-thunk';
import { createStore, compose, applyMiddleware } from "redux";
import persistState from 'redux-localstorage'
import { composeWithDevTools } from 'redux-devtools-extension';

import rootReducer from "./reducers";

const enhancer = composeWithDevTools(
  applyMiddleware(thunkMiddleware),
  persistState(["from", "until", "labels"]),
)

export default createStore(rootReducer, enhancer);
