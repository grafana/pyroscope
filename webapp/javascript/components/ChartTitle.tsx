import React from 'react';
import Color from 'color';
import clsx from 'clsx';
import styles from './ChartTitle.module.scss';

const unitsToFlamegraphTitle = {
  objects: 'Total number of objects in RAM',
  goroutines: 'Total number of goroutines',
  bytes: 'Total amount of RAM',
  samples: 'Total CPU time',
  lock_nanoseconds: 'Total time spent waiting on locks',
  lock_samples: 'Total number of contended locks',
  baseline: 'Baseline Flamegraph',
  comparison: 'Comparison Flamegraph',
  diff: 'Baseline vs. Comparison Diff',
  trace_samples: 'Total aggregated span duration',
  '': '',
};

interface ChartTitleProps {
  color?: Color;
  titleKey?: keyof typeof unitsToFlamegraphTitle;
  className?: string;
}

export default function ChartTitle({
  color,
  titleKey = '',
  className,
}: ChartTitleProps) {
  return (
    <div className={clsx([styles.chartTitle, className])}>
      {color && (
        <span
          className={styles.colorReference}
          style={{ backgroundColor: color.rgb().toString() }}
        />
      )}
      <p className={styles.title}>{unitsToFlamegraphTitle[titleKey]}</p>
    </div>
  );
}
