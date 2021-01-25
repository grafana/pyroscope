import React from "react";
import { connect } from "react-redux";
import "react-dom";

import Spinner from "react-svg-spinner";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faFile } from "@fortawesome/free-solid-svg-icons";
import { faGithub } from "@fortawesome/free-brands-svg-icons";
import classNames from "classnames";
import DateRangePicker from "./DateRangePicker";
import RefreshButton from "./RefreshButton";
import SlackIcon from "./SlackIcon";
import Label from "./Label";
import NameSelector from "./NameSelector";

import { fetchNames } from "../redux/actions";

function Header(props) {
  const { areNamesLoading, isJSONLoading, labels } = props;
  return (
    <div className="navbar">
      <h1 className="logo" />
      <div
        className={classNames("labels", {
          visible: !areNamesLoading,
        })}
      >
        <NameSelector names={labels.map((i) => i.value)} />
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
      <div className="navbar-links">
        <span className="navbar-link">
          <FontAwesomeIcon icon={faFile} />
          &nbsp;
          <a rel="noreferrer" target="_blank" href="https://pyroscope.io/docs">
            Docs
          </a>
        </span>
        <span className="navbar-link">
          <SlackIcon />
          &nbsp;
          <a rel="noreferrer" target="_blank" href="https://pyroscope.io/slack">
            Slack
          </a>
        </span>
        <span className="navbar-link">
          <FontAwesomeIcon icon={faGithub} />
          &nbsp;
          <a
            rel="noreferrer"
            target="_blank"
            href="https://github.com/pyroscope-io/pyroscope"
          >
            GitHub
          </a>
        </span>
      </div>
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
