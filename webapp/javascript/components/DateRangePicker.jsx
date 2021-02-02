import React, { useState } from "react";
import { useDispatch, useSelector } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faClock } from "@fortawesome/free-solid-svg-icons";
import DatePicker from "react-datepicker";
import OutsideClickHandler from "react-outside-click-handler";
import { setDateRange } from "../redux/actions";
import humanReadableRange from "../util/formatDate";

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
function DateRangePicker() {
  const dispatch = useDispatch();
  const from = useSelector((state) => state.from);
  const until = useSelector((state) => state.until);
  const [opened, setOpened] = useState(false);
  const readableDateForm = humanReadableRange(until, from);

  const updateFrom = (from) => {
    dispatch(setDateRange(from, until));
  };

  const updateUntil = (until) => {
    dispatch(setDateRange(from, until));
  };

  const updateDateRange = () => {
    dispatch(setDateRange(from, until));
  };

  const toggleDropdown = () => {
    setOpened(!opened);
  };

  const hideDropdown = () => {
    setOpened(false);
  };

  const selectPreset = ({ from, until }) => {
    dispatch(setDateRange(from, until));
    setOpened(false);
  };

  return (
    <div className={opened ? "drp-container opened" : "drp-container"}>
      <OutsideClickHandler onOutsideClick={hideDropdown}>
        <button
          type="button"
          className="btn drp-button"
          onClick={toggleDropdown}
        >
          <FontAwesomeIcon icon={faClock} />
          <span>{readableDateForm.range}</span>
        </button>
        <div className="drp-dropdown">
          <h4>Quick Presets</h4>
          <div className="drp-presets">
            {defaultPresets.map((arr, i) => (
              <div key={`preset-${i + 1}`} className="drp-preset-column">
                {arr.map((x) => (
                  <button
                    type="button"
                    className={`drp-preset ${
                      x.label === readableDateForm.range ? "active" : ""
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
          <div className="drp-label">From</div>
          <div className="drp-calendar-input-group">
            <DatePicker
              className="followed-by-btn"
              showTimeSelect
              dateFormat="MMM d, yyyy h:mm aa"
              onChange={(date) => updateFrom(date / 1000)}
              onBlur={() => updateDateRange()}
              selected={readableDateForm.from}
            />
            <button
              type="button"
              className="drp-calendar-btn btn"
              onClick={updateDateRange}
            >
              <FontAwesomeIcon icon={faClock} />
              Update
            </button>
          </div>
          <div className="drp-label">To</div>
          <div className="drp-calendar-input-group">
            <DatePicker
              className="followed-by-btn"
              showTimeSelect
              dateFormat="MMM d, yyyy h:mm aa"
              onChange={(date) => updateUntil(date / 1000)}
              onBlur={() => updateDateRange()}
              selected={readableDateForm.until}
            />
            <button
              type="button"
              className="drp-calendar-btn btn"
              onClick={updateDateRange}
            >
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
