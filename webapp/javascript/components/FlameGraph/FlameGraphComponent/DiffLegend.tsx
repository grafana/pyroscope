import React from 'react';
import { colorFromPercentage } from './color';
import styles from './DiffLegend.module.css';

export default function DiffLegend() {
  const values = [100, 80, 60, 40, 20, 10, 0, -10, -20, -40, -60, -80, -100];

  return (
    <div
      className={`row ${styles['flamegraph-legend']}`}
      data-testid="flamegraph-legend"
    >
      <div className={styles['flamegraph-legend-list']}>
        {values.map((v) => (
          <div
            key={v}
            className={styles['flamegraph-legend-item']}
            style={{
              backgroundColor: colorFromPercentage(v, 0.8).string(),
            }}
          >
            {v > 0 ? '+' : ''}
            {v}%
          </div>
        ))}
      </div>
    </div>
  );
}
