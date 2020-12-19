import ReactDOM from "react-dom"
import React from "react"

import { Provider } from "react-redux";
import store from "./redux/store";

import { ShortcutProvider } from 'react-keybind'

// import PyroscopeApp from "./components/PyroscopeApp";
import PyroscopeApp2 from "./components/PyroscopeApp2";


ReactDOM.render(
  <Provider store={store}>
    <ShortcutProvider>
      <PyroscopeApp2/>
    </ShortcutProvider>
  </Provider>,
  document.getElementById('root')
);
