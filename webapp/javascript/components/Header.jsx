import React from 'react';
import { connect } from "react-redux";
import "react-dom";

import Spinner from "react-svg-spinner";

import DateRangePicker from "./DateRangePicker";
import DownloadButton from './DownloadButton';
import RefreshButton from './RefreshButton';
import SlackIcon from './SlackIcon';
import Label from "./Label";
import NameSelector from "./NameSelector";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFile } from '@fortawesome/free-solid-svg-icons';
import { faGithub } from '@fortawesome/free-brands-svg-icons';

import classNames from "classnames";

import { fetchNames } from "../redux/actions";

class Header extends React.Component {
  constructor(props) {
    super(props);
  }



  render() {
    return <div className="navbar">
      <h1 className="logo"></h1>
      <div className={
        classNames("labels", { visible: !this.props.areNamesLoading })
      }>
        <NameSelector/>
        {this.props.labels.filter(x => x.name !== "__name__").map(function(label) {
          return <Label key={label.name} label={label}></Label>;
        })}
      </div>
      {/* <div className={
        classNames("navbar-spinner-container", {
          visible: this.props.areNamesLoading
        })
      }>
        <Spinner color="rgba(255,255,255,0.6)" size="20px"/>
      </div> */}
      {/* <LabelsFilter /> */}
      <div className="navbar-space-filler"></div>
      <div className="navbar-links">
        <span className="navbar-link"><FontAwesomeIcon icon={faFile} />&nbsp;<a target="_blank" href="https://pyroscope.io/docs">Docs</a></span>
        <span className="navbar-link"><SlackIcon/>&nbsp;<a target="_blank" href="https://pyroscope.io/slack">Slack</a></span>
        <span className="navbar-link"><FontAwesomeIcon icon={faGithub} />&nbsp;<a target="_blank" href="https://github.com/pyroscope-io/pyroscope">GitHub</a></span>
      </div>
      <div className={
        classNames("navbar-spinner-container", {
          visible: this.props.isJSONLoading
        })
      }>
        <Spinner color="rgba(255,255,255,0.6)" size="20px"/>
      </div>
      &nbsp;
      <RefreshButton/>
      &nbsp;
      <DateRangePicker />
    </div>
  }
}

export default connect(
  (x) => x,
  { fetchNames }
)(Header);
