import ReactDOM from "react-dom"
import React from "react"

import { Provider } from "react-redux";
import store from "./redux/store";

import ProfileApp from "./components/ProfileApp";


ReactDOM.render(
  <Provider store={store}>
    <ProfileApp/>
  </Provider>,
  document.getElementById('root') as HTMLElement
);
