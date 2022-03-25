import React, { useState, useEffect } from 'react';
import { isAfter, isSameSecond } from 'date-fns';
import DatePicker from 'react-datepicker';
import Button from '@webapp/ui/Button';
import { formatAsOBject } from '@webapp/util/formatDate';

interface CustomDatePickerProps {
  from: string;
  until: string;
  onSubmit: (from: string, until: string) => void;
}
function CustomDatePicker({ from, until, onSubmit }: CustomDatePickerProps) {
  const [warning, setWarning] = useState(false);
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

    onSubmit(
      Math.round(selectedDate.from.getTime() / 1000).toString(),
      Math.round(selectedDate.until.getTime() / 1000).toString()
    );
    return setWarning(false);
  };

  // Since 'from' and 'until' are the source of truth
  // Since our component state back when they change
  useEffect(() => {
    setSelectedDate({
      ...selectedDate,
      from: formatAsOBject(from),
      until: formatAsOBject(until),
    });
  }, [from, until]);

  const selectFromAsDate = selectedDate.from;
  const selectUntilAsDate = selectedDate.until;

  return (
    <div className="drp-custom">
      <h4>Custom Date Range</h4>
      <div className="from">
        <label htmlFor="datepicker-from">From: </label>
        <DatePicker
          id="datepicker-from"
          selected={selectFromAsDate}
          onChange={(date) => {
            if (date) {
              setSelectedDate({ ...selectedDate, from: date });
            }
          }}
          selectsStart
          showTimeSelect
          startDate={selectFromAsDate}
          dateFormat="yyyy-MM-dd hh:mm aa"
        />
      </div>
      <div className="until">
        <label htmlFor="datepicker-until">Until: </label>
        <DatePicker
          id="datepicker-until"
          selected={selectUntilAsDate}
          onChange={(date) => {
            if (date) {
              setSelectedDate({ ...selectedDate, until: date });
            }
          }}
          selectsEnd
          showTimeSelect
          startDate={selectFromAsDate}
          endDate={selectUntilAsDate}
          minDate={selectFromAsDate}
          dateFormat="yyyy-MM-dd hh:mm aa"
        />
      </div>
      {warning && <p style={{ color: 'red' }}>Warning: invalid date Range</p>}

      <Button type="submit" kind="primary" onClick={() => updateDateRange()}>
        Apply range
      </Button>
    </div>
  );
}

export default CustomDatePicker;
