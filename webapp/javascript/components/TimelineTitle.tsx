import React from 'react';
import Color from 'color';

import styles from './TimelineTitle.module.scss';

interface TimelineTitleProps {
  color?: Color;
  title: string;
}

export default function TimelineTitle({ color, title }: TimelineTitleProps) {
  return (
    <div className={styles.timelineTitle}>
      {color && (
        <span
          className={styles.colorReference}
          style={{ backgroundColor: color.rgb().toString() }}
        />
      )}
      <p className={styles.title}>{title}</p>
    </div>
  );
}
