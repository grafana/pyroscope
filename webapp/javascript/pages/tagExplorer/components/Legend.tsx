import React from 'react';

import type { TimelineGroupData } from '@webapp/components/TimelineChartWrapper';
import styles from './Legend.module.scss';

interface LegendProps {
  groups: TimelineGroupData[];
  handleGroupByTagValueChange: (groupByTagValue: string) => void;
}

function Legend({ groups, handleGroupByTagValueChange }: LegendProps) {
  return (
    <div data-testid="legend" className={styles.legend}>
      {groups.map(({ tagName, color }) => (
        <div
          data-testid="legend-item"
          className={styles.tagName}
          key={tagName}
          onClick={() => handleGroupByTagValueChange(tagName)}
        >
          <span
            data-testid="legend-item-color"
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
