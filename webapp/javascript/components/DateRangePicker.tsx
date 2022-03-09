import React, { useState } from 'react';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  setDateRange,
  selectContinuousState,
} from '@pyroscope/redux/reducers/continuous';
import Button from '@ui/Button';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';
import OutsideClickHandler from 'react-outside-click-handler';
import CustomDatePicker from './CustomDatePicker';
import CheckIcon from './CheckIcon';
import { readableRange } from '../util/formatDate';

const defaultPresets = [
  [
    { label: 'Last 5 minutes', from: 'now-5m', until: 'now' },
    { label: 'Last 15 minutes', from: 'now-15m', until: 'now' },
    { label: 'Last 30 minutes', from: 'now-30m', until: 'now' },
    { label: 'Last 1 hour', from: 'now-1h', until: 'now' },
    { label: 'Last 3 hours', from: 'now-3h', until: 'now' },
    { label: 'Last 6 hours', from: 'now-6h', until: 'now' },
    { label: 'Last 12 hours', from: 'now-12h', until: 'now' },
    { label: 'Last 24 hours', from: 'now-24h', until: 'now' },
  ],
  [
    { label: 'Last 2 days', from: 'now-2d', until: 'now' },
    { label: 'Last 7 days', from: 'now-7d', until: 'now' },
    { label: 'Last 30 days', from: 'now-30d', until: 'now' },
    { label: 'Last 90 days', from: 'now-90d', until: 'now' },
    { label: 'Last 6 months', from: 'now-6M', until: 'now' },
    { label: 'Last 1 year', from: 'now-1y', until: 'now' },
    { label: 'Last 2 years', from: 'now-2y', until: 'now' },
    { label: 'Last 5 years', from: 'now-5y', until: 'now' },
  ],
];

function findPreset(from: string, until = 'now') {
  return defaultPresets
    .flat()
    .filter((a) => a.until === until)
    .find((a) => from === a.from);
}

function dateToLabel(from: string, until: string) {
  const preset = findPreset(from, until);

  if (preset) {
    return preset.label;
  }

  return readableRange(from, until);
}

function DateRangePicker() {
  const dispatch = useAppDispatch();
  const { from, until } = useAppSelector(selectContinuousState);

  const [opened, setOpened] = useState(false);
  const [range, setRange] = useState();

  const toggleDropdown = () => {
    setOpened(!opened);
  };

  const hideDropdown = () => {
    setOpened(false);
  };
  const selectPreset = ({ from, until }) => {
    dispatch(setDateRange({ from, until }));
    setOpened(false);
  };

  return (
    <div className={opened ? 'drp-container opened' : 'drp-container'}>
      <OutsideClickHandler onOutsideClick={hideDropdown}>
        <Button icon={faClock} onClick={toggleDropdown}>
          {dateToLabel(from, until)}
        </Button>
        <div className="drp-dropdown">
          <div className="drp-quick-presets">
            <h4>Quick Presets</h4>
            <div className="drp-presets">
              {defaultPresets.map((arr, i) => (
                <div key={`preset-${i + 1}`} className="drp-preset-column">
                  {arr.map((x) => (
                    <button
                      type="button"
                      className={`drp-preset ${
                        x.label === range ? 'active' : ''
                      }`}
                      key={x.label}
                      onClick={() => selectPreset(x)}
                    >
                      {x.label}
                      {x.label === range ? <CheckIcon /> : false}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          </div>
          <CustomDatePicker
            from={from}
            until={until}
            setRange={setRange}
            dispatch={dispatch}
            setDateRange={setDateRange}
          />
        </div>
      </OutsideClickHandler>
    </div>
  );
}

export default DateRangePicker;
