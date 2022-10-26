import React from 'react';
import Button from '@webapp/ui/Button';
import { TimelineData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { useSync } from './useSync';
import StatusMessage from '@webapp/ui/StatusMessage';
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
    <StatusMessage
      type="warning"
      message={title}
      action={
        <div className={styles.buttons}>
          <Button
            data-testid="sync-ignore-button"
            onClick={onIgnore}
            className={styles.ignoreButton}
          >
            Ignore
          </Button>
          <Button
            data-testid="sync-button"
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
