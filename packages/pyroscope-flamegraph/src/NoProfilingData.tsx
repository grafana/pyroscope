import React from 'react';

import styles from './NoProfilingData.module.scss';

export default function NoProfilingData() {
  return (
    <div className={styles.noProfilingData}>
      <span>
        No profiling data available for this application / time range.
      </span>
    </div>
  );
}
