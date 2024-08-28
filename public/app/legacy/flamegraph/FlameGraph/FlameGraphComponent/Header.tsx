import React from 'react';
import { Flamebearer } from '@pyroscope/legacy/models';
import styles from './Header.module.css';
import { FlamegraphPalette } from './colorPalette';
import { DiffLegendPaletteDropdown } from './DiffLegendPaletteDropdown';

interface HeaderProps {
  format: Flamebearer['format'];
  units: Flamebearer['units'];

  palette: FlamegraphPalette;
  setPalette: (p: FlamegraphPalette) => void;
  toolbarVisible?: boolean;
}

const unitsToFlamegraphTitle = new Map(
  Object.entries({
    objects: 'number of objects in RAM per function',
    goroutines: 'number of goroutines',
    bytes: 'amount of RAM per function',
    samples: 'CPU time per function',
    lock_nanoseconds: 'time spent waiting on locks per function',
    lock_samples: 'number of contended locks per function',
    trace_samples: 'aggregated span duration',
    exceptions: 'number of exceptions thrown',
    unknown: '',
  })
);

export default function Header(props: HeaderProps) {
  const { format, units, palette, setPalette, toolbarVisible } = props;

  const getTitle = () => {
    switch (format) {
      case 'single': {
        return (
          <div>
            <div
              className={`${styles.row} ${styles.flamegraphTitle}`}
              role="heading"
              aria-level={2}
            >
              {unitsToFlamegraphTitle.has(units) && (
                <>Frame width represents {unitsToFlamegraphTitle.get(units)}</>
              )}
            </div>
          </div>
        );
      }

      case 'double': {
        return (
          <>
            <DiffLegendPaletteDropdown
              palette={palette}
              onChange={setPalette}
            />
          </>
        );
      }

      default:
        throw new Error(`unexpected format ${format}`);
    }
  };

  const title = toolbarVisible ? getTitle() : null;

  return (
    <div className={styles.flamegraphHeader}>
      <div>{title}</div>
    </div>
  );
}
