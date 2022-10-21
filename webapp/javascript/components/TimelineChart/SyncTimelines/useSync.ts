import { centerTimelineData } from '@webapp/components/TimelineChart/centerTimelineData';
import useTimelines from '@webapp/hooks/timeline.hook';
import {
  actions,
  selectContinuousState,
} from '@webapp/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { useEffect, useState } from 'react';
import { markingsFromSelection, Selection } from '../markings';

const timeOffset = 5000;

const getTitle = (leftInRange: boolean, rightInRange: boolean) => {
  if (!leftInRange && !rightInRange) {
    return 'Warning: Baseline and Comparison timeline selections are out of range';
  }
  if (!rightInRange) {
    return 'Warning: Comparison timeline selection is out of range';
  }
  return 'Warning: Baseline timeline selection is out of range';
};

export function useSync() {
  const dispatch = useAppDispatch();
  const { leftTimeline } = useTimelines();
  const [isIgnoring, setIgnoring] = useState(false);
  const { leftFrom, rightFrom, leftUntil, rightUntil, from, until } =
    useAppSelector(selectContinuousState);

  useEffect(() => {
    if (isIgnoring) {
      setIgnoring(false);
    }
  }, [leftFrom, rightFrom, leftUntil, rightUntil, from, until]);

  const leftSelectionMarkings = markingsFromSelection('single', {
    from: leftFrom,
    to: leftUntil,
  } as Selection);
  const rightSelectionMarkings = markingsFromSelection('single', {
    from: rightFrom,
    to: rightUntil,
  } as Selection);

  const centeredData = centerTimelineData(leftTimeline);

  const timelineFrom = centeredData?.[0]?.[0];
  const timelineTo = centeredData?.[centeredData?.length - 1]?.[0];

  const leftSelectionFrom = leftSelectionMarkings?.[0]?.xaxis?.from;
  const leftSelectionTo = leftSelectionMarkings?.[0]?.xaxis?.to;

  const rightSelectionFrom = rightSelectionMarkings?.[0]?.xaxis?.from;
  const rightSelectionTo = rightSelectionMarkings?.[0]?.xaxis?.to;

  const offset = [leftFrom, rightFrom, leftUntil, rightUntil, from, until].some(
    (p) => String(p).startsWith('now')
  )
    ? timeOffset
    : 1;

  const leftInRange =
    leftSelectionFrom + timeOffset >= timelineFrom &&
    leftSelectionTo - timeOffset <= timelineTo;

  const rightInRange =
    rightSelectionFrom + timeOffset >= timelineFrom &&
    rightSelectionTo - timeOffset <= timelineTo;

  const selectionsLimits = [
    leftSelectionFrom,
    leftSelectionTo,
    rightSelectionFrom,
    rightSelectionTo,
  ];

  const selectionMin = Math.min(...selectionsLimits);
  const selectionMax = Math.max(...selectionsLimits);

  const timeIsRelative = [
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
    from,
    until,
  ].every((t) => t.startsWith('now'));

  const onSync = () => {
    dispatch(
      actions.setLeft({
        from: String(leftSelectionFrom),
        until: String(leftSelectionTo),
      })
    );
    dispatch(
      actions.setRight({
        from: String(rightSelectionFrom),
        until: String(rightSelectionTo),
      })
    );
    dispatch(
      actions.setFromAndUntil({
        from: String(selectionMin - offset),
        until: String(selectionMax + offset),
      })
    );
  };

  return {
    isWarningHidden:
      !leftTimeline.data?.samples.length ||
      (leftInRange && rightInRange) ||
      timeIsRelative ||
      isIgnoring,
    title: getTitle(leftInRange, rightInRange),
    onIgnore: () => setIgnoring(true),
    onSync,
  };
}
