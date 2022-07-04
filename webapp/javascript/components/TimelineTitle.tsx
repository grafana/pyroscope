import React from 'react';
import Color from 'color';

import styles from './TimelineTitle.module.scss';

interface TimelineTitleProps {
  color?: Color;
  title: string;
}

export default function TimelineTitle({ color, title }: TimelineTitleProps) {
  return (
    <>
      {color && <h1>{color}</h1>}
      <p className={styles.timelineTitle}>{title}</p>
    </>
  );
}
