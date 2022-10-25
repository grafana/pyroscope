import React from 'react';
import { TimelineData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { useSync } from './useSync';
import styles from './styles.module.scss';

interface SyncTimelinesProps {
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

function SyncTimelines({
  timeline,
  leftSelection,
  rightSelection,
  onSync,
}: SyncTimelinesProps) {
  const { isWarningHidden, onIgnore, title, onSyncClick } = useSync({
    timeline,
    leftSelection,
    rightSelection,
    onSync,
  });

  if (isWarningHidden) {
    return null;
  }

  return (
    <div className={styles.wrapper}>
      {title}
      <div className={styles.buttons}>
        <button
          onClick={onIgnore}
          type="button"
          className={styles.ignoreButton}
        >
          Ignore
        </button>
        <button
          onClick={onSyncClick}
          type="button"
          className={styles.syncButton}
        >
          Sync Timelines
        </button>
      </div>
    </div>
  );
}

export default SyncTimelines;
