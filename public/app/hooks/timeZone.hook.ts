/* eslint-disable prefer-template */
/* eslint-disable @typescript-eslint/restrict-plus-operands */
import { useEffect, useMemo } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  changeTimeZoneOffset,
  selectTimezoneOffset,
} from '@pyroscope/redux/reducers/ui';

export default function useTimeZone() {
  const dispatch = useAppDispatch();
  const selectedOffset = useAppSelector(selectTimezoneOffset);
  const offset = new Date().getTimezoneOffset();

  useEffect(() => {
    if (typeof selectedOffset !== 'number') {
      dispatch(changeTimeZoneOffset(offset));
    }
  }, [dispatch, offset, selectedOffset]);

  const browserTimeLabel = useMemo(() => {
    const absOffset = Math.abs(offset);

    return `Browser Time (UTC${offset < 0 ? '+' : '-'}${
      ('00' + Math.floor(absOffset / 60)).slice(-2) +
      ':' +
      ('00' + (absOffset % 60)).slice(-2)
    })`;
  }, [offset]);

  return {
    offset: typeof selectedOffset !== 'number' ? offset : selectedOffset,
    options: [
      {
        label: browserTimeLabel,
        value: offset,
        key: 'local',
      },
      {
        label: 'Default (UTC)',
        value: 0,
        key: 'utc',
      },
    ],
    changeTimeZoneOffset: (value: number) =>
      dispatch(changeTimeZoneOffset(value)),
  };
}
