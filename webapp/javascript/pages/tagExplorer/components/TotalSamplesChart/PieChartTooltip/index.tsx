import React from 'react';
import styles from './styles.module.scss';

export interface PieChartTooltipProps {
  label?: string;
  percent?: number;
  value?: number;
}

const PieChartTooltip = (props: PieChartTooltipProps) => {
  return (
    <div className={styles.wrapper}>
      <div>{props?.label || ' '}</div>
      <div>
        Total samples: <span className={styles.bold}>{props?.value}</span>
      </div>
      <div>
        Percentage:{' '}
        <span className={styles.bold}>
          {Number(props?.percent).toFixed(2)}%
        </span>
      </div>
    </div>
  );
};

export default PieChartTooltip;
