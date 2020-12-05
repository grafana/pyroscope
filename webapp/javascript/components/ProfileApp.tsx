import React from 'react';
import { connect } from "react-redux";
import "react-dom"
import Spinner from "react-svg-spinner";
import DateRangePicker from "./DateRangePicker";
import RefreshButton from './RefreshButton';
import SVGRenderer from "./SVGRenderer";
import LabelsFilter from "./LabelsFilter";
import Label from "./Label";

import classNames from "classnames";

class ProfileApp extends React.Component {
  constructor(props) {
    super(props);
  }

  render() {
    return (
      <div className="todo-app">
        <div className="navbar">
          <h1 className="logo"></h1>
          <div className="labels">
            {this.props.labels.map(function(label) {
              return <Label key={label.name} label={label}></Label>;
            })}
          </div>
          <LabelsFilter />
          <div className="navbar-space-filler"></div>
          <div className={
            classNames("navbar-spinner-container", {
              visible: this.props.isDataLoading
            })
          }>
            <Spinner color="rgba(255,255,255,0.6)" size="20px"/>
          </div>
          <RefreshButton />
          &nbsp;
          <DateRangePicker />
        </div>
        <SVGRenderer />
      </div>
    );
  }
}


export default connect(
  (x) => x,
  {}
)(ProfileApp);
