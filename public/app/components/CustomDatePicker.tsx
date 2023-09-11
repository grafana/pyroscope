import React, { useState, useEffect } from 'react';
import { isAfter, isSameSecond } from 'date-fns';
import DatePicker from 'react-datepicker';
import Button from '@pyroscope/ui/Button';
import { formatAsOBject, getUTCdate } from '@pyroscope/util/formatDate';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import Select from '@pyroscope/ui/Select';
import TextField from '@pyroscope/ui/Form/TextField';
import styles from './CustomDatePicker.module.scss';

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
      // ...selectedDate, -- we are replacing all values in the object, no need to keep the old one
      from: formatAsOBject(from),
      until: formatAsOBject(until),
    });
  }, [from, until]);

  const selectFromAsDate = getUTCdate(selectedDate.from, offset);
  const selectUntilAsDate = getUTCdate(selectedDate.until, offset);

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
        <DatePicker
          id="datepicker-from"
          selected={selectFromAsDate}
          onChange={(date) => onDateChange(date, 'from')}
          selectsStart
          showTimeSelect
          startDate={selectFromAsDate}
          dateFormat="yyyy-MM-dd hh:mm aa"
          customInput={
            <TextField
              className={styles.datePickerInput}
              label="From:"
              variant="light"
            />
          }
        />
      </div>
      <div className="until">
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
          customInput={
            <TextField
              className={styles.datePickerInput}
              label="Until:"
              variant="light"
            />
          }
        />
      </div>
      {warning && <p style={{ color: 'red' }}>Warning: invalid date Range</p>}

      <Button type="submit" kind="secondary" onClick={() => updateDateRange()}>
        Apply range
      </Button>

      <div style={{ marginTop: 10 }}>
        <label htmlFor="select-timezone">Time Zone: </label>
        <Select
          ariaLabel="select-timezone"
          onChange={(e) => changeTimeZoneOffset(Number(e.target.value))}
          id="select-timezone"
          value={String(offset)}
          disabled={timeZoneOptions.every((o) => o.value === 0)}
          className={styles.timezoneSelect}
        >
          {timeZoneOptions.map((o) => (
            <option key={o.key} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      </div>
    </div>
  );
}

export default CustomDatePicker;
