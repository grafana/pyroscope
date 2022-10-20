import { centerTimelineData } from '@webapp/components/TimelineChart/centerTimelineData';
import useTimelines from '@webapp/hooks/timeline.hook';
import {
  actions,
  selectContinuousState,
} from '@webapp/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { markingsFromSelection, Selection } from '../markings';

const timeOffset = 5000;

export function useSync() {
  const dispatch = useAppDispatch();
  const { leftTimeline } = useTimelines();
  const { leftFrom, rightFrom, leftUntil, rightUntil, from, until } =
    useAppSelector(selectContinuousState);
  const centeredData = centerTimelineData(leftTimeline);

  const leftSelectionMarkings = markingsFromSelection('single', {
    from: leftFrom,
    to: leftUntil,
  } as Selection);
  const rightSelectionMarkings = markingsFromSelection('single', {
    from: rightFrom,
    to: rightUntil,
  } as Selection);

  const timelineFrom = centeredData?.[0]?.[0];
  const timelineTo = centeredData?.[centeredData?.length - 1]?.[0];

  const leftSelectionFrom = leftSelectionMarkings?.[0]?.xaxis?.from;
  const leftSelectionTo = leftSelectionMarkings?.[0]?.xaxis?.to;

  const rightSelectionFrom = rightSelectionMarkings?.[0]?.xaxis?.from;
  const rightSelectionTo = rightSelectionMarkings?.[0]?.xaxis?.to;

  const selectionsLimits = [
    leftSelectionFrom,
    leftSelectionTo,
    rightSelectionFrom,
    rightSelectionTo,
  ];

  const selectionMin = Math.min(...selectionsLimits);
  const selectionMax = Math.max(...selectionsLimits);

  const isFullyRelativeTime = [
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
    from,
    until,
  ].every((t) => t.startsWith('now'));

  const minInRange = selectionMin + timeOffset >= timelineFrom;
  const maxInRange = selectionMax - timeOffset <= timelineTo;

  const timeSpan = [
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
    from,
    until,
  ].some((t) => t.startsWith('now'))
    ? timeOffset
    : 1;

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
        from: String(selectionMin - timeSpan),
        until: String(selectionMax + timeSpan),
      })
    );
  };

  return {
    isWarningHidden:
      (minInRange && maxInRange) ||
      !leftTimeline.data?.samples.length ||
      isFullyRelativeTime,
    onSync,
  };
}
