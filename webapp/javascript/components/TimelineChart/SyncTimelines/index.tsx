import React from 'react';
import { actions } from '@webapp/redux/reducers/continuous';
import { useAppDispatch } from '@webapp/redux/hooks';
import { checkSelectionInRange } from '@webapp/components/TimelineChart/SyncTimelines/checkSelectionInRange';
import { unitsToFlamegraphTitle } from '@webapp/components/TimelineTitle';
import { TimelineData, centerTimelineData } from '../centerTimelineData';
import { Selection } from '../markings';

import styles from './styles.module.scss';

interface SyncTimelinesProps {
  titleKey: 'baseline' | 'comparison';
  timeline: TimelineData;
  selection: {
    from: Selection['from'];
    to: Selection['to'];
  };
}
function SyncTimelines({ timeline, selection, titleKey }: SyncTimelinesProps) {
  const dispatch = useAppDispatch();

  if (!timeline.data?.samples.length) {
    return null;
  }

  const inRange = checkSelectionInRange({
    timeline,
    selection,
  });

  const onClick = () => {
    const centeredData = centerTimelineData(timeline);
    const timelineFrom = centeredData?.[0]?.[0];
    const timelineTo = centeredData?.[centeredData?.length - 1]?.[0];

    const diff = timelineTo - timelineFrom;

    const action =
      titleKey === 'baseline'
        ? actions.setLeft({
            from: String(timelineFrom),
            until: String(timelineFrom + diff / 2),
          })
        : actions.setRight({
            from: String(timelineFrom + diff / 2),
            until: String(timelineTo),
          });

    dispatch(action);
  };

  if (inRange) {
    return null;
  }

  return (
    <div className={styles.wrapper}>
      Warning: {unitsToFlamegraphTitle[titleKey]} selection is out of range
      <button onClick={onClick} type="button" className={styles.syncButton}>
        Sync Timelines
      </button>
    </div>
  );
}

export default SyncTimelines;
