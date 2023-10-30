import { useEffect, useState } from 'react';
import { centerTimelineData } from '@pyroscope/components/TimelineChart/centerTimelineData';
import { TimelineData } from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import { getSelectionBoundaries } from '@pyroscope/components/TimelineChart/SyncTimelines/getSelectionBoundaries';
import { Selection } from '../markings';

interface UseSyncParams {
  timeline: TimelineData;
  leftSelection: {
    from: string;
    to: string;
  };
  rightSelection: {
    from: string;
    to: string;
  };
  onSync: (from: string, until: string) => void;
}

const selectionOffset = 5000;

export const getTitle = (leftInRange: boolean, rightInRange: boolean) => {
  if (!leftInRange && !rightInRange) {
    return 'Warning: Baseline and Comparison timeline selections are out of range';
  }
  if (!rightInRange) {
    return 'Warning: Comparison timeline selection is out of range';
  }
  return 'Warning: Baseline timeline selection is out of range';
};

const isInRange = (
  from: number,
  to: number,
  selectionFrom: number,
  selectionTo: number
) => {
  const timeOffset = (to - from) * 0.1;

  return selectionFrom + timeOffset >= from && selectionTo - timeOffset <= to;
};

export function useSync({
  timeline,
  leftSelection,
  rightSelection,
  onSync,
}: UseSyncParams) {
  const [isIgnoring, setIgnoring] = useState(false);

  useEffect(() => {
    setIgnoring(false);
  }, [leftSelection, rightSelection, timeline]);

  const { from: leftFrom, to: leftTo } = getSelectionBoundaries(
    leftSelection as Selection
  );

  const { from: rightFrom, to: rightTo } = getSelectionBoundaries(
    rightSelection as Selection
  );

  const centeredData = centerTimelineData(timeline);

  const timelineFrom = centeredData?.[0]?.[0];
  const timelineTo = centeredData?.[centeredData?.length - 1]?.[0];

  const isLeftInRange = isInRange(timelineFrom, timelineTo, leftFrom, leftTo);
  const isRightInRange = isInRange(
    timelineFrom,
    timelineTo,
    rightFrom,
    rightTo
  );

  const onSyncClick = () => {
    const selectionsLimits = [leftFrom, leftTo, rightFrom, rightTo];
    const selectionMin = Math.min(...selectionsLimits);
    const selectionMax = Math.max(...selectionsLimits);
    // when some of selection is in relative time (now, now-1h etc.), we have to extend detecting time buffer
    // 1) to prevent falsy detections
    // 2) to workaraund pecularity that when we change selection we don't refetch main timeline
    const offset = [
      leftSelection.from,
      rightSelection.from,
      leftSelection.to,
      rightSelection.to,
    ].some((p) => String(p).startsWith('now'))
      ? selectionOffset
      : 1;

    onSync(String(selectionMin - offset), String(selectionMax + offset));
  };

  return {
    isWarningHidden:
      !timeline.data?.samples.length ||
      (isLeftInRange && isRightInRange) ||
      isIgnoring,
    title: getTitle(isLeftInRange, isRightInRange),
    onIgnore: () => setIgnoring(true),
    onSyncClick,
  };
}
