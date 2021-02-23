import ReactDOM from "react-dom";
import React from "react";

import { Provider } from "react-redux";
import { ShortcutProvider } from "react-keybind";
import { Router, Switch, Route } from "react-router-dom";
import store from "./redux/store";

import PyroscopeApp from "./components/PyroscopeApp";
import ComparisonApp from "./components/ComparisonApp";
import Sidebar from "./components/Sidebar";

import history from "./util/history";

ReactDOM.render(
  <Provider store={store}>
    <Router history={history}>
      <ShortcutProvider>
        <Sidebar />
        <Switch>
          <Route exact path="/">
            <PyroscopeApp />
          </Route>
          <Route path="/comparison">
            <ComparisonApp />
          </Route>
        </Switch>
      </ShortcutProvider>
    </Router>
  </Provider>,
  document.getElementById("root")
);
