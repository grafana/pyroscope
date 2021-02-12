import React, { useState, useEffect } from "react";
import moment from "moment";
import { useSelector } from "react-redux";
import DatePicker from "react-datepicker";
import { readableRange, formatAsOBject } from "../util/formatDate";

function CustomDatePicker({
  setRange,
  dispatch,
  setDateRange,
  storePreviousDateRange,
}) {
  const from = useSelector((state) => state.from);
  const until = useSelector((state) => state.until);
  const previousDateRange = useSelector((state) => state.previousDateRange);
  const [warning, setWarning] = useState(false);
  const [selectedDate, setSelectedDate] = useState({
    from: formatAsOBject(from),
    until: formatAsOBject(until),
  });

  const updateDateRange = () => {
    if (moment(selectedDate.from).isSameOrAfter(selectedDate.until)) {
      return setWarning(true);
    }
    dispatch(storePreviousDateRange({ from: from, until: until }));
    dispatch(
      setDateRange(
        Math.round(selectedDate.from / 1000),
        Math.round(selectedDate.until / 1000)
      )
    );
    return setWarning(false);
  };

  useEffect(() => {
    setSelectedDate({
      ...selectedDate,
      from: formatAsOBject(from),
      until: formatAsOBject(until),
    });
    setRange(readableRange(from, until));
  }, [from, until]);

  return (
    <div>
      <h4>Custom Date Range</h4>
      <div className="form">
        <p style={{ marginBottom: "10px", color: "white" }}>From: </p>
        <DatePicker
          selected={selectedDate.from}
          onChange={(date) => {
            setSelectedDate({ ...selectedDate, from: date });
          }}
          selectsStart
          showTimeSelect
          startDate={selectedDate.from}
          dateFormat="yyyy-MM-dd hh:mm aa"
        />
      </div>
      <div className="until">
        <p style={{ marginBottom: "10px", color: "white" }}>Until: </p>
        <DatePicker
          selected={selectedDate.until}
          onChange={(date) => {
            setSelectedDate({ ...selectedDate, until: date });
          }}
          selectsEnd
          showTimeSelect
          startDate={selectedDate.from}
          endDate={selectedDate.until}
          minDate={selectedDate.from}
          dateFormat="yyyy-MM-dd hh:mm aa"
        />
      </div>
      {warning && <p style={{ color: "red" }}>Warning: invalid date Range</p>}

      <button
        style={{
          marginTop: "20px",
          backgroundColor: "#2ECC40",
          color: "white",
        }}
        type="submit"
        className="btn"
        onClick={() => updateDateRange()}
      >
        Apply range
      </button>
      <button
        style={{
          marginTop: "20px",
          marginLeft: "20px",
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
    </div>
  );
}

export default CustomDatePicker;
