import React from "react";
import { connect, useDispatch, useSelector } from "react-redux";
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
import {
  fetchNames,
  setDateRange,
  storePreviousDateRange,
} from "../redux/actions";

function Header(props) {
  const from = useSelector((state) => state.from);
  const until = useSelector((state) => state.until);
  const previousDateRange = useSelector((state) => state.previousDateRange);
  const dispatch = useDispatch();
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
      <button
        type="button"
        style={{
          marginRight: "6px",
          backgroundColor: "#C3170D",
          color: "white",
        }}
        type="submit"
        className="btn"
        onClick={() => {
          dispatch(storePreviousDateRange({ from: from, until: until }));
          dispatch(
            setDateRange(previousDateRange.from, previousDateRange.until)
          );
        }}
      >
        Previous Time
      </button>
      <DateRangePicker />
    </div>
  );
}

export default connect((x) => x, { fetchNames })(Header);
