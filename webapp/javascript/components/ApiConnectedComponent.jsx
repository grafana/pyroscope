import React from 'react';
import { connect } from "react-redux";
import "react-dom";

import Modal from "react-modal";
import {withShortcut} from "react-keybind";

// import FlameGraphRenderer from "./FlameGraphRenderer";
import FlameGraphRenderer2 from "./FlameGraphRenderer2";
// import TimelineChart from "./TimelineChart";
import TimelineChart2 from "./TimelineChart2";
import ShortcutsModal from "./ShortcutsModal";
import Header from "./Header";
import Footer from "./Footer";

import { receiveNames, receiveJSON } from "../redux/actions";
import { bindActionCreators } from "redux";
import { buildRenderURL, fetchJSON, fetchNames } from '../util/update_requests';

class ApiConnectedComponent extends React.Component {
  constructor() {
    super()
    
    this.fetchJSON = fetchJSON.bind(this);
    this.fetchNames = fetchNames.bind(this);
    this.buildRenderURL = buildRenderURL.bind(this);
  }


}


export default ApiConnectedComponent;