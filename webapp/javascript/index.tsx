import ReactDOM from "react-dom"
import React from "react"

import { Provider } from "react-redux";
import store from "./redux/store";

import PyroscopeApp from "./components/PyroscopeApp";


ReactDOM.render(
  <Provider store={store}>
    <PyroscopeApp/>
  </Provider>,
  document.getElementById('root') as HTMLElement
);
