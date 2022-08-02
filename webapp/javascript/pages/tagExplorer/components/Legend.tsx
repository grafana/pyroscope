import React from 'react';

import type { TimelineGroupData } from '@webapp/components/TimelineChartWrapper';
import styles from './Legend.module.scss';

interface LegendProps {
  groups: TimelineGroupData[];
  handleGroupByTagValueChange: (groupByTagValue: string) => void;
}

function Legend({ groups, handleGroupByTagValueChange }: LegendProps) {
  return (
    <div className={styles.legend}>
      {groups.map(({ tagName, color }) => (
        <div
          className={styles.tagName}
          key={tagName}
          onClick={() => handleGroupByTagValueChange(tagName)}
        >
          <span
            className={styles.tagColor}
            style={{ backgroundColor: color?.toString() }}
          />
          <span>{tagName}</span>
        </div>
      ))}
    </div>
  );
}

export default Legend;
