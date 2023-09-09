import React, { ReactNode } from 'react';
import Color from 'color';
import clsx from 'clsx';
import styles from './ChartTitle.module.scss';

const chartTitleKeys = {
  objects: 'Total number of objects in RAM',
  goroutines: 'Total number of goroutines',
  bytes: 'Total amount of RAM',
  samples: 'Total CPU time',
  lock_nanoseconds: 'Total time spent waiting on locks',
  lock_samples: 'Total number of contended locks',
  diff: 'Baseline vs. Comparison Diff',
  trace_samples: 'Total aggregated span duration',
  exceptions: 'Total number of exceptions thrown',
  unknown: '',

  baseline: 'Baseline Flamegraph',
  comparison: 'Comparison Flamegraph',
  selection_included: 'Selection-included Exemplar Flamegraph',
  selection_excluded: 'Selection-excluded Exemplar Flamegraph',
};

type ChartTitleKey = keyof typeof chartTitleKeys;

interface ChartTitleProps {
  children?: ReactNode;
  className?: string;
  color?: Color;
  icon?: ReactNode;
  postfix?: ReactNode;
  titleKey?: keyof typeof chartTitleKeys;
}

export function getChartTitle(key: ChartTitleKey) {
  return chartTitleKeys[key];
}

export default function ChartTitle({
  children,
  className,
  color,
  icon,
  postfix,
  titleKey = 'unknown',
}: ChartTitleProps) {
  return (
    <div className={clsx([styles.chartTitle, className])}>
      {(icon || color) && (
        <span
          className={clsx(styles.colorOrIcon, icon && styles.icon)}
          style={
            !icon && color ? { backgroundColor: color.rgb().toString() } : {}
          }
        >
          {icon}
        </span>
      )}
      <p className={styles.title}>{children || getChartTitle(titleKey)}</p>
      {postfix}
    </div>
  );
}
