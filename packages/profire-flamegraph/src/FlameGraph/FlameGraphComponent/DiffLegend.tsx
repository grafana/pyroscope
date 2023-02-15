import React from 'react';
import { NewDiffColor } from './color';
import { FlamegraphPalette } from './colorPalette';
import styles from './DiffLegend.module.css';

export type sizeMode = 'small' | 'large';
interface DiffLegendProps {
  palette: FlamegraphPalette;
  showMode: sizeMode;
}

export default function DiffLegend(props: DiffLegendProps) {
  const { palette, showMode } = props;
  const values = decideLegend(showMode);

  const color = NewDiffColor(palette);

  return (
    <div
      data-testid="flamegraph-legend"
      className={`${styles['flamegraph-legend']} ${styles['flamegraph-legend-list']}`}
    >
      {values.map((v) => (
        <div
          key={v}
          className={styles['flamegraph-legend-item']}
          style={{
            backgroundColor: color(v).rgb().toString(),
          }}
        >
          {v > 0 ? '+' : ''}
          {v}%
        </div>
      ))}
    </div>
  );
}

function decideLegend(showMode: sizeMode) {
  switch (showMode) {
    case 'large': {
      return [-100, -80, -60, -40, -20, -10, 0, 10, 20, 40, 60, 80, 100];
    }

    case 'small': {
      return [-100, -40, -20, 0, 20, 40, 100];
    }

    default:
      throw new Error(`Unsupported ${showMode}`);
  }
}
