import React, { useState } from "react";
import { useDispatch, useSelector } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faClock } from "@fortawesome/free-solid-svg-icons";

import OutsideClickHandler from "react-outside-click-handler";
import moment from "moment";
import { setDateRange } from "../redux/actions";

const defaultPresets = [
  [
    { label: "Last 5 minutes", from: "now-5m", until: "now" },
    { label: "Last 15 minutes", from: "now-15m", until: "now" },
    { label: "Last 30 minutes", from: "now-30m", until: "now" },
    { label: "Last 1 hour", from: "now-1h", until: "now" },
    { label: "Last 3 hours", from: "now-3h", until: "now" },
    { label: "Last 6 hours", from: "now-6h", until: "now" },
    { label: "Last 12 hours", from: "now-12h", until: "now" },
    { label: "Last 24 hours", from: "now-24h", until: "now" },
  ],
  [
    { label: "Last 2 days", from: "now-2d", until: "now" },
    { label: "Last 7 days", from: "now-7d", until: "now" },
    { label: "Last 30 days", from: "now-30d", until: "now" },
    { label: "Last 90 days", from: "now-90d", until: "now" },
    { label: "Last 6 months", from: "now-6M", until: "now" },
    { label: "Last 1 year", from: "now-1y", until: "now" },
    { label: "Last 2 years", from: "now-2y", until: "now" },
    { label: "Last 5 years", from: "now-5y", until: "now" },
  ],
];

const multiplierMapping = {
  s: "second",
  m: "minute",
  h: "hour",
  d: "day",
  w: "week",
  M: "month",
  y: "year",
};

function DateRangePicker() {
  const dispatch = useDispatch();
  const from = useSelector((state) => state.from);
  const until = useSelector((state) => state.until);

  const initialState = {
    // so the idea with this is that we don't want to send from and until back to the state
    //   until the user clicks some button. This is why these are stored in state here.
    from,
    until,
    opened: false,
  };
  const [state, setState] = useState(initialState);
  const [presets, setPresets] = useState(defaultPresets);

  const updateFrom = (from) => {
    setState({ from });
  };

  const updateUntil = (until) => {
    setState({ until });
  };

  const updateDateRange = () => {
    dispatch(setDateRange(from, until));
  };

  const humanReadableRange = () => {
    if (until === "now") {
      const m = from.match(/^now-(?<number>\d+)(?<multiplier>\D+)$/);
      if (m && multiplierMapping[m.groups.multiplier]) {
        let multiplier = multiplierMapping[m.groups.multiplier];
        if (m.groups.number > 1) {
          multiplier += "s";
        }
        return `Last ${m.groups.number} ${multiplier}`;
      }
    }
    return `${moment(from * 1000).format("lll")} – ${moment(
      until * 1000
    ).format("lll")}`;
    // return from + " to " +until;
  };

  const showDropdown = () => {
    setState({
      opened: !state.opened,
    });
  };

  const selectPreset = ({ from, until }) => {
    dispatch(setDateRange(from, until));
    hideDropdown();
  };

  const hideDropdown = () => {
    setState({
      opened: false,
    });
  };

  return (
    <div className={state.opened ? "drp-container opened" : "drp-container"}>
      <OutsideClickHandler onOutsideClick={hideDropdown}>
        <button className="btn drp-button" onClick={showDropdown}>
          <FontAwesomeIcon icon={faClock} />
          <span>{humanReadableRange()}</span>
        </button>
        <div className="drp-dropdown">
          <h4>Quick Presets</h4>
          <div className="drp-presets">
            {presets.map((arr, i) => (
              <div key={`preset-${i + 1}`} className="drp-preset-column">
                {arr.map((x) => (
                  <button
                    className={`drp-preset ${
                      x.label === humanReadableRange() ? "active" : ""
                    }`}
                    key={x.label}
                    onClick={() => selectPreset(x)}
                  >
                    {x.label}
                  </button>
                ))}
              </div>
            ))}
          </div>
          <h4>Custom Date Range</h4>
          <div className="drp-calendar-input-group">
            <input
              className="followed-by-btn"
              onChange={(e) => updateFrom(e.target.value)}
              onBlur={updateDateRange}
              value={from}
            />
            <button className="drp-calendar-btn btn" onClick={updateDateRange}>
              <FontAwesomeIcon icon={faClock} />
              Update
            </button>
          </div>
          <div className="drp-calendar-input-group">
            <input
              className="followed-by-btn"
              onChange={(e) => updateUntil(e.target.value)}
              onBlur={updateDateRange}
              value={until}
            />
            <button className="drp-calendar-btn btn" onClick={updateDateRange}>
              <FontAwesomeIcon icon={faClock} />
              Update
            </button>
          </div>
        </div>
      </OutsideClickHandler>
    </div>
  );
}

export default DateRangePicker;
