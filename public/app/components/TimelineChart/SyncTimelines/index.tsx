import React from 'react';
import Button from '@pyroscope/ui/Button';
import { TimelineData } from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import StatusMessage from '@pyroscope/ui/StatusMessage';
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
  comparisonModeActive?: boolean;
  isDataLoading?: boolean;
}

function SyncTimelines({
  timeline,
  leftSelection,
  rightSelection,
  onSync,
  comparisonModeActive = false,
  isDataLoading = false,
}: SyncTimelinesProps) {
  const { isWarningHidden, onIgnore, title, onSyncClick } = useSync({
    timeline,
    leftSelection,
    rightSelection,
    onSync,
  });

  if (isWarningHidden || comparisonModeActive || isDataLoading) {
    return null;
  }

  return (
    <StatusMessage
      type="warning"
      message={title}
      action={
        <div className={styles.buttons}>
          <Button
            kind="outline"
            onClick={onIgnore}
            className={styles.ignoreButton}
          >
            Ignore
          </Button>
          <Button
            kind="outline"
            onClick={onSyncClick}
            className={styles.syncButton}
          >
            Sync Timelines
          </Button>
        </div>
      }
    />
  );
}

export default SyncTimelines;
