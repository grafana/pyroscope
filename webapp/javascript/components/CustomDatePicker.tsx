import React, { useState, useEffect } from 'react';
import { isAfter, isSameSecond } from 'date-fns';
import DatePicker from 'react-datepicker';
import Button from '@webapp/ui/Button';
import { formatAsOBject, getUTCdate } from '@webapp/util/formatDate';
import useTimeZone from '@webapp/hooks/timeZone.hook';

interface CustomDatePickerProps {
  from: string;
  until: string;
  onSubmit: (from: string, until: string) => void;
}
function CustomDatePicker({ from, until, onSubmit }: CustomDatePickerProps) {
  const {
    options: timeZoneOptions,
    changeTimeZoneOffset,
    offset,
  } = useTimeZone();
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

  const selectFromAsDate = getUTCdate(selectedDate.from, offset === 0);
  const selectUntilAsDate = getUTCdate(selectedDate.until, offset === 0);

  const onDateChange = (date: Date | null, area: 'from' | 'until') => {
    if (date) {
      setSelectedDate({
        ...selectedDate,
        [area]:
          offset === 0
            ? new Date(
                date.getTime() + date.getTimezoneOffset() * 60 * 1000 * -1
              )
            : date,
      });
    }
  };

  return (
    <div className="drp-custom">
      <h4>Custom Date Range</h4>
      <div className="from">
        <label htmlFor="datepicker-from">From: </label>
        <DatePicker
          id="datepicker-from"
          selected={selectFromAsDate}
          onChange={(date) => onDateChange(date, 'from')}
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
          onChange={(date) => onDateChange(date, 'until')}
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

      <div style={{ marginTop: 10 }}>
        <label htmlFor="select-timezone">Time Zone: </label>
        <select
          onChange={(e) => changeTimeZoneOffset(Number(e.target.value))}
          id="select-timezone"
          value={String(offset)}
          disabled={timeZoneOptions.every((o) => o.value === 0)}
        >
          {timeZoneOptions.map((o) => (
            <option key={o.key} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}

export default CustomDatePicker;
