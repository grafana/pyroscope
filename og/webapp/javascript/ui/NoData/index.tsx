import React from 'react';

import styles from './NoData.module.scss';

export default function NoData() {
  return (
    <div data-testid="no-data" className={styles.noData}>
      <span>No data available</span>
    </div>
  );
}
