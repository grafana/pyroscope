import React, { ReactNode } from 'react';
import Color from 'color';
import clsx from 'clsx';
import styles from './ChartTitle.module.scss';

import profileMetrics from '../constants/profile-metrics.json';

const typeToDescriptionMap: Record<string, string> = Object.values(
  profileMetrics
).reduce((acc, { type, description }) => ({ ...acc, [type]: description }), {});

const chartTitleKeys = {
  ...typeToDescriptionMap,
  exception: typeToDescriptionMap.exceptions, // alias
  unknown: '',

  baseline: 'Baseline time range',
  comparison: 'Comparison time range',
  selection_included: 'Selection-included Exemplar Flamegraph',
  selection_excluded: 'Selection-excluded Exemplar Flamegraph',
};

type ChartTitleKey = keyof typeof chartTitleKeys;

export interface ChartTitleProps {
  children?: ReactNode;
  className?: string;
  color?: Color;
  icon?: ReactNode;
  postfix?: ReactNode;
  titleKey?: ChartTitleKey;
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
