import React from 'react';

import type { TimelineGroupData } from './TimelineChartWrapper';
import styles from './Legend.module.scss';

interface LegendProps {
  groups: TimelineGroupData[];
}

function Legend({ groups }: LegendProps) {
  return (
    <div className={styles.legend}>
      {groups.map(
        ({ tagName, color }) =>
          tagName !== '*' && (
            <div className={styles.tag} key={tagName}>
              <span
                className={styles.color}
                style={{ backgroundColor: color?.toString() }}
              />
              <span>{tagName}</span>
            </div>
          )
      )}
    </div>
  );
}

export default Legend;
