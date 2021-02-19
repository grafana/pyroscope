import ReactDOM from "react-dom";
import React from "react";

import { Provider } from "react-redux";
import { ShortcutProvider } from "react-keybind";
import store from "./redux/store";

import PyroscopeApp from "./components/PyroscopeApp";
import Sidebar from "./components/Sidebar";

import history from "./util/history";

import {
  Router,
  Switch,
  Route,
  Link
} from "react-router-dom";

function ComingSoon() {
  return <h2 style={{    
    "display": "flex",
    "flexDirection": "column",
    "marginLeft": "100px"}}>Coming soon</h2>;
}

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
            <ComingSoon />
          </Route>
        </Switch>
      </ShortcutProvider>
    </Router>
  </Provider>,
  document.getElementById("root")
);
