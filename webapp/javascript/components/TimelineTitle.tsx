import React from 'react';
import Color from 'color';

import styles from './TimelineTitle.module.scss';

const timelineTitles = {
  total: 'Total CPU Time',
  baseline: 'Baseline Flamegraph',
  comparison: 'Comparison Flamegraph',
  diff: 'Baseline vs. Comparison Diff',
};

interface TimelineTitleProps {
  color?: Color;
  titleKey: keyof typeof timelineTitles;
}

export default function TimelineTitle({ color, titleKey }: TimelineTitleProps) {
  return (
    <div className={styles.timelineTitle}>
      {color && (
        <span
          className={styles.colorReference}
          style={{ backgroundColor: color.rgb().toString() }}
        />
      )}
      <p className={styles.title}>{timelineTitles[titleKey]}</p>
    </div>
  );
}
