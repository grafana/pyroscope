import React, { useState, useEffect } from 'react';
import { isAfter, isSameSecond } from 'date-fns';
import { useSelector } from 'react-redux';
import DatePicker from 'react-datepicker';
import Button from '@ui/Button';
import { RootState } from '@pyroscope/redux/store';
import { readableRange, formatAsOBject } from '../util/formatDate';

function CustomDatePicker({ setRange, dispatch, setDateRange }) {
  const from = useSelector((state: RootState) => state.root.from);
  const until = useSelector((state: RootState) => state.root.until);
  const [warning, setWarning] = useState(false);
  const [isFutureDate, setFutureDate] = useState(false);
  const [selectedDate, setSelectedDate] = useState({
    from: formatAsOBject(from),
    until: formatAsOBject(until),
  });

  const updateDateRange = () => {
    if (
      isSameSecond(selectedDate.from, selectedDate.until) ||
      isAfter(selectedDate.from, selectedDate.until)
    ) {
      return setWarning(true);
    }
    
    dispatch(
      setDateRange(
        Math.round(selectedDate.from / 1000),
        Math.round(selectedDate.until / 1000)
      )
    );
    return setWarning(false);
  };

  const checkFutureDate= (date: Date) => {
    let dateToday = new Date(Date.now())
    
    if(isAfter(date, dateToday)) {
      return setFutureDate(true)
    }
    return setFutureDate(false)
  }

  useEffect(() => {
    setSelectedDate({
      ...selectedDate,
      from: formatAsOBject(from),
      until: formatAsOBject(until),
    });

    setRange(readableRange(from, until));
  }, [from, until]);

  return (
    <div className="drp-custom">
      <h4>Custom Date Range</h4>
      <div className="from">
        <label htmlFor="datepicker-from">From: </label>
        <DatePicker
          id="datepicker-from"
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
        <label htmlFor="datepicker-until">Until: </label>
        <DatePicker
          id="datepicker-until"
          selected={selectedDate.until}
          onChange={(date) => {
            checkFutureDate(date);
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
      {warning && <p style={{ color: 'red' }}>Warning: invalid date Range</p>}
      {isFutureDate && <p style={{ color: 'yellow' }}>Warning: Until can not be a future date</p>}

      <Button type="submit" kind="primary" disabled={isFutureDate ? true: false} onClick={() => updateDateRange()}>
        Apply range
      </Button>
    </div>
  );
}

export default CustomDatePicker;
