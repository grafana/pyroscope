import ReactDOM from "react-dom"
import React from "react"

import { Provider } from "react-redux";
import store from "./redux/store";

import { ShortcutProvider } from 'react-keybind'

import PyroscopeApp from "./components/PyroscopeApp";


ReactDOM.render(
  <Provider store={store}>
    <ShortcutProvider>
      <PyroscopeApp/>
    </ShortcutProvider>
  </Provider>,
  document.getElementById('root')
);
