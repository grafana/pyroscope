import { areIntervalsOverlapping } from 'date-fns';
import {
  centerTimelineData,
  TimelineData,
} from '@webapp/components/TimelineChart/centerTimelineData';
import { markingsFromSelection, Selection } from '../markings';

export function checkSelectionInRange({
  timeline,
  selection,
}: {
  timeline: TimelineData;
  selection: {
    from: Selection['from'];
    to: Selection['to'];
  };
}): boolean {
  const centeredData = centerTimelineData(timeline);
  const selectionMarkings = markingsFromSelection(
    'single',
    selection as Selection
  );
  const timelineFrom = centeredData?.[0]?.[0];
  const timelineTo = centeredData?.[centeredData?.length - 1]?.[0];
  const selectionFrom = selectionMarkings?.[0]?.xaxis?.from;
  const selectionTo = selectionMarkings?.[0]?.xaxis?.to;

  const fullRange = {
    start: new Date(timelineFrom),
    end: new Date(timelineTo),
  };
  const selectionRange = {
    start: new Date(selectionFrom),
    end: new Date(selectionTo),
  };

  return areIntervalsOverlapping(fullRange, selectionRange);
}
