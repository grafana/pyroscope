import { useEffect, useState } from 'react';
import { centerTimelineData } from '@webapp/components/TimelineChart/centerTimelineData';
import { TimelineData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { markingsFromSelection, Selection } from '../markings';

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

const timeOffset = 5 * 60 * 1000;
const selectionOffset = 5000;

const getTitle = (leftInRange: boolean, rightInRange: boolean) => {
  if (!leftInRange && !rightInRange) {
    return 'Warning: Baseline and Comparison timeline selections are out of range';
  }
  if (!rightInRange) {
    return 'Warning: Comparison timeline selection is out of range';
  }
  return 'Warning: Baseline timeline selection is out of range';
};

export function useSync({
  timeline,
  leftSelection,
  rightSelection,
  onSync,
}: UseSyncParams) {
  const [isIgnoring, setIgnoring] = useState(false);

  useEffect(() => {
    if (isIgnoring) {
      setIgnoring(false);
    }
  }, [leftSelection, rightSelection, timeline]);

  const [
    {
      xaxis: { from: leftMarkingsFrom, to: leftMarkingsTo },
    },
  ] = markingsFromSelection('single', {
    ...leftSelection,
  } as Selection);
  const [
    {
      xaxis: { from: rightMarkingsFrom, to: rightMarkingsTo },
    },
  ] = markingsFromSelection('single', {
    ...rightSelection,
  } as Selection);

  const centeredData = centerTimelineData(timeline);

  const timelineFrom = centeredData?.[0]?.[0];
  const timelineTo = centeredData?.[centeredData?.length - 1]?.[0];

  const leftInRange =
    leftMarkingsFrom + timeOffset >= timelineFrom &&
    leftMarkingsTo - timeOffset <= timelineTo;

  const rightInRange =
    rightMarkingsFrom + timeOffset >= timelineFrom &&
    rightMarkingsTo - timeOffset <= timelineTo;

  const onSyncClick = () => {
    const selectionsLimits = [
      leftMarkingsFrom,
      leftMarkingsTo,
      rightMarkingsFrom,
      rightMarkingsTo,
    ];
    const selectionMin = Math.min(...selectionsLimits);
    const selectionMax = Math.max(...selectionsLimits);

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
      (leftInRange && rightInRange) ||
      isIgnoring,
    title: getTitle(leftInRange, rightInRange),
    onIgnore: () => setIgnoring(true),
    onSyncClick,
  };
}
