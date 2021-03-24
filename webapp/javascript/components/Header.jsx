import React from "react";
import { connect } from "react-redux";
import "react-dom";

import Spinner from "react-svg-spinner";

import classNames from "classnames";
import DateRangePicker from "./DateRangePicker";
import RefreshButton from "./RefreshButton";
import Label from "./Label";
import NameSelector from "./NameSelector";

import { fetchNames } from "../redux/actions";

function Header(props) {
  const { areNamesLoading, isJSONLoading, labels } = props;
  return (
    <div className="navbar">
      <div
        className={classNames("labels", {
          visible: !areNamesLoading,
        })}
      >
        <NameSelector />
        {labels
          .filter((x) => x.name !== "__name__")
          .map((label) => (
            <Label key={label.name} label={label} />
          ))}
      </div>
      {/* <div className={
      classNames("navbar-spinner-container", {
        visible: this.props.areNamesLoading
      })
    }>
      <Spinner color="rgba(255,255,255,0.6)" size="20px"/>
    </div> */}
      {/* <LabelsFilter /> */}
      <div className="navbar-space-filler" />
      <div
        className={classNames("navbar-spinner-container", {
          visible: isJSONLoading,
        })}
      >
        <Spinner color="rgba(255,255,255,0.6)" size="20px" />
      </div>
      &nbsp;
      <RefreshButton />
      &nbsp;
      <DateRangePicker />
    </div>
  );
}

export default connect((x) => x, { fetchNames })(Header);
