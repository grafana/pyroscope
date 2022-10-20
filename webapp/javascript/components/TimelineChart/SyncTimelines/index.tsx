import React from 'react';
import { useSync } from './useSync';

import styles from './styles.module.scss';

function SyncTimelines() {
  const { isWarningHidden, onSync } = useSync();

  if (isWarningHidden) {
    return null;
  }

  return (
    <div className={styles.wrapper}>
      Warning: Main Timeline selection is out of range
      <button onClick={onSync} type="button" className={styles.syncButton}>
        Sync Timelines
      </button>
    </div>
  );
}

export default SyncTimelines;
