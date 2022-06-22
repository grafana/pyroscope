import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  fetchSideTimelines,
  selectContinuousState,
  selectTimelineSidesData,
} from '@webapp/redux/reducers/continuous';
import Color from 'color';

// Purple
export const leftColor = Color('rgb(208, 102, 212)');
// Blue
export const rightColor = Color('rgb(19, 152, 246)');

export default function useTimelines() {
  const dispatch = useAppDispatch();
  const {
    from,
    until,
    refreshToken,
    maxNodes,

    leftQuery,
    rightQuery,
  } = useAppSelector(selectContinuousState);
  const timelines = useAppSelector(selectTimelineSidesData);

  // Only reload timelines when an item that affects a timeline has changed
  useEffect(() => {
    dispatch(fetchSideTimelines(null));
  }, [from, until, refreshToken, maxNodes, leftQuery, rightQuery]);

  const leftTimeline = {
    color: leftColor.rgb().toString(),
    data: timelines.left,
  };

  const rightTimeline = {
    color: rightColor.rgb().toString(),
    data: timelines.right,
  };
  return {
    leftTimeline,
    rightTimeline,
  };
}
