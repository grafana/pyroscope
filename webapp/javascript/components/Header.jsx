import React from "react";
import { connect } from "react-redux";
import "react-dom";

import Spinner from "react-svg-spinner";

import classNames from "classnames";
import DateRangePicker from "./DateRangePicker";
import RefreshButton from "./RefreshButton";
import NameSelector from "./NameSelector";
import TagsBar from "./TagsBar";

import { fetchNames } from "../redux/actions";

function Header(props) {
  const { areNamesLoading, isJSONLoading } = props;
  return (
    <>
      <div className="navbar">
        <div
          className={classNames("labels", {
            visible: !areNamesLoading,
          })}
        >
          <NameSelector />
        </div>
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
      <TagsBar />
    </>
  );
}

export default connect((x) => x, { fetchNames })(Header);
