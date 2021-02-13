import ReactDOM from "react-dom";
import React from "react";

import { Provider } from "react-redux";
import { ShortcutProvider } from "react-keybind";
import store from "./redux/store";

import PyroscopeApp from "./components/PyroscopeApp";
import Sidebar from "./components/Sidebar";

ReactDOM.render(
  <Provider store={store}>
    <ShortcutProvider>
      <Sidebar />
      <PyroscopeApp />
    </ShortcutProvider>
  </Provider>,
  document.getElementById("root")
);
