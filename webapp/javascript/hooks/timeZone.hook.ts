/* eslint-disable prefer-template */
import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  changeTimeZoneOffset,
  selectTimezoneOffset,
} from '@webapp/redux/reducers/ui';

export default function useTimeZone() {
  const dispatch = useAppDispatch();
  const selectedOffset = useAppSelector(selectTimezoneOffset);
  const offset = new Date().getTimezoneOffset();

  useEffect(() => {
    if (typeof selectedOffset !== 'number') {
      dispatch(changeTimeZoneOffset(offset));
    }
  }, []);

  return {
    offset: selectedOffset,
    options: [
      {
        label: `Browser Time (UTC${offset < 0 ? '+' : '-'}${
          ('00' + Math.floor(Math.abs(offset) / 60)).slice(-2) +
          ':' +
          ('00' + (Math.abs(offset) % 60)).slice(-2)
        })`,
        value: offset,
        key: 'local',
      },
      {
        label: 'Default (UTC)',
        value: 0,
        key: 'utc',
      },
    ],
    changeTimeZoneOffset: (value: any) => dispatch(changeTimeZoneOffset(value)),
  };
}
