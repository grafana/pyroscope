import React from 'react';
import { Flamebearer, unitsDescription } from '@pyroscope/models/src';
import styles from './Header.module.css';
import { FlamegraphPalette } from './colorPalette';
import { DiffLegendPaletteDropdown } from './DiffLegendPaletteDropdown';

interface HeaderProps {
  format: Flamebearer['format'];
  units: Flamebearer['units'];

  palette: FlamegraphPalette;
  setPalette: (p: FlamegraphPalette) => void;
  //  ExportData: React.ReactElement | JSX.Element | JSX.Element[] | null;
  //  ExportData: JSX.Element | null;
  //  ExportData: React.ElementType | null;
  //  ExportData?: JSX.Element;
  ExportData?: React.ReactNode;
  toolbarVisible?: boolean;
}
export default function Header(props: HeaderProps) {
  const {
    format,
    units,
    ExportData = <></>,
    palette,
    setPalette,
    toolbarVisible,
  } = props;

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
              {unitsDescription[units] && (
                <>Frame width represents {unitsDescription[units]}</>
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
      {ExportData ? <div className={styles.buttons}>{ExportData}</div> : <></>}
    </div>
  );
}
