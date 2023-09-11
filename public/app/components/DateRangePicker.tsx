import React, { useState } from 'react';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  setDateRange,
  selectContinuousState,
  actions,
} from '@pyroscope/redux/reducers/continuous';
import cx from 'classnames';
import Button from '@pyroscope/ui/Button';
import { readableRange } from '@pyroscope/util/formatDate';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';
import OutsideClickHandler from 'react-outside-click-handler';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import CustomDatePicker from './CustomDatePicker';
import CheckIcon from './CheckIcon';

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
  ],
];

function findPreset(from: string, until = 'now') {
  return defaultPresets
    .flat()
    .filter((a) => a.until === until)
    .find((a) => from === a.from);
}

function dateToLabel(from: string, until: string, offsetInMinutes: number) {
  const preset = findPreset(from, until);

  if (preset) {
    return preset.label;
  }

  return readableRange(from, until, offsetInMinutes);
}

function DateRangePicker() {
  const dispatch = useAppDispatch();
  const { offset } = useTimeZone();
  const {
    from,
    until,
    comparisonView: { comparisonMode },
  } = useAppSelector(selectContinuousState);
  const [opened, setOpened] = useState(false);

  const toggleDropdown = () => {
    setOpened(!opened);
  };

  const hideDropdown = () => {
    setOpened(false);
  };
  const selectPreset = ({ from, until }: { from: string; until: string }) => {
    dispatch(setDateRange({ from, until }));
    setOpened(false);

    if (comparisonMode.active) {
      dispatch(
        actions.setComparisonMode({
          ...comparisonMode,
          active: false,
        })
      );
    }
  };

  const isPresetSelected = (preset: (typeof defaultPresets)[0][0]) => {
    return preset.label === dateToLabel(from, until, offset);
  };

  const handleChangeDataRange = (from: string, until: string) => {
    dispatch(setDateRange({ from, until }));

    if (comparisonMode.active) {
      dispatch(
        actions.setComparisonMode({
          ...comparisonMode,
          active: false,
        })
      );
    }
  };

  return (
    <div className={opened ? 'drp-container opened' : 'drp-container'}>
      <OutsideClickHandler onOutsideClick={hideDropdown}>
        <Button
          data-testid="time-dropdown-button"
          icon={faClock}
          onClick={toggleDropdown}
        >
          {dateToLabel(from, until, offset)}
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
                      className={cx(
                        'drp-preset',
                        isPresetSelected(x) && 'active'
                      )}
                      key={x.label}
                      onClick={() => selectPreset(x)}
                    >
                      {x.label}
                      {isPresetSelected(x) ? <CheckIcon /> : false}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          </div>
          <CustomDatePicker
            from={from}
            until={until}
            onSubmit={handleChangeDataRange}
          />
        </div>
      </OutsideClickHandler>
    </div>
  );
}

export default DateRangePicker;
