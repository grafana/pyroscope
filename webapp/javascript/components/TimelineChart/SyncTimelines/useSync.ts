import { centerTimelineData } from '@webapp/components/TimelineChart/centerTimelineData';
import useTimelines from '@webapp/hooks/timeline.hook';
import {
  actions,
  selectContinuousState,
} from '@webapp/redux/reducers/continuous';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { markingsFromSelection, Selection } from '../markings';

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

  const min = Math.min(...selectionsLimits);
  const max = Math.max(...selectionsLimits);

  const isFullyRelativeTime = [
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
    from,
    until,
  ].every((t) => t.startsWith('now'));

  return {
    isWarningHidden:
      (min >= timelineFrom && max <= timelineTo) ||
      !leftTimeline.data?.samples.length ||
      isFullyRelativeTime,
    onSync: () => {
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
          from: String(min - 5000),
          until: String(max + 5000),
        })
      );
    },
  };
}
