import React from 'react';
import { Flamebearer } from '@models/flamebearer';
import DiffLegend from './DiffLegend';
import styles from './Header.module.css';

export default function Header({
  format,
  units,
  ExportData,
}: {
  format: Flamebearer['format'];
  units: Flamebearer['units'];
  ExportData: () => React.ReactElement;
}) {
  const unitsToFlamegraphTitle = {
    objects: 'amount of objects in RAM per function',
    bytes: 'amount of RAM per function',
    samples: 'CPU time per function',
  };

  const getTitle = () => {
    switch (format) {
      case 'single': {
        return (
          <div>
            <div
              className={`${styles.row} ${styles['flamegraph-title']}`}
              role="heading"
              aria-level={2}
            >
              {unitsToFlamegraphTitle[units] && (
                <>Frame width represents {unitsToFlamegraphTitle[units]}</>
              )}
            </div>
          </div>
        );
      }

      case 'double': {
        return (
          <div>
            <div className={styles.row} role="heading" aria-level={2}>
              Base graph: left - Comparison graph: right
            </div>
            <DiffLegend />
          </div>
        );
      }

      default:
        throw new Error(`unexpected format ${format}`);
    }
  };

  const title = getTitle();

  return (
    <div className={styles['flamegraph-header']}>
      <div>{title}</div>
      <ExportData />
    </div>
  );
}
