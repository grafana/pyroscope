import React from 'react';
import { useSync } from './useSync';

import styles from './styles.module.scss';

function SyncTimelines() {
  const { isWarningHidden, onSync, onIgnore, title } = useSync();

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
        <button onClick={onSync} type="button" className={styles.syncButton}>
          Sync Timelines
        </button>
      </div>
    </div>
  );
}

export default SyncTimelines;
