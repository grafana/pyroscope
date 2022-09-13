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
      <div className={styles.title}>{props?.label || ' '}</div>
      <table className={styles.table}>
        <thead>
          <tr>
            <td>Total samples</td>
            <td>Percentage</td>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>{props?.value}</td>
            <td>{Number(props?.percent).toFixed(2)}%</td>
          </tr>
        </tbody>
      </table>
    </div>
  );
};

export default PieChartTooltip;
