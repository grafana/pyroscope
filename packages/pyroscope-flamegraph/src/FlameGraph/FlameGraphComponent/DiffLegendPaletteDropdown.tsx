import React, { useMemo } from 'react';
import cx from 'classnames';
import useResizeObserver from '@react-hook/resize-observer';
import {
  ColorBlindPalette,
  DefaultPalette,
  DefaultLightPalette,
  ColorBlindLightPalette,
  FlamegraphPalette,
} from './colorPalette';
import DiffLegend from './DiffLegend';
import CheckIcon from './CheckIcon';
// Until we migrate ui to its own package this should do it
// eslint-disable-next-line
import Dropdown, { MenuItem, MenuButton } from '@webapp/ui/Dropdown';
// eslint-disable-next-line
import dropdownStyles from '@webapp/ui/Dropdown.module.scss';

import styles from './DiffLegendPaletteDropdown.module.css';

interface DiffLegendPaletteDropdownProps {
  palette: FlamegraphPalette;
  onChange: (p: FlamegraphPalette) => void;
  colorMode?: 'light' | 'dark';
}

export const DiffLegendPaletteDropdown = (
  props: DiffLegendPaletteDropdownProps
) => {
  const { palette = DefaultPalette, onChange, colorMode } = props;
  const legendRef = React.useRef<HTMLDivElement>(null);
  const showMode = useSizeMode(legendRef);

  const paletteList = useMemo(
    () =>
      colorMode === 'light'
        ? [DefaultLightPalette, ColorBlindLightPalette]
        : [DefaultPalette, ColorBlindPalette],
    [colorMode]
  );

  return (
    <>
      <div className={styles.row} role="heading" aria-level={2}>
        <p style={{ color: palette.goodColor.rgb().string() }}>(-) Removed</p>
        <p style={{ color: palette.badColor.rgb().string() }}>Added (+)</p>
      </div>

      <div ref={legendRef} className={styles.dropdownWrapper}>
        <Dropdown
          label="Select a palette"
          menuButton={
            <MenuButton
              className={cx(
                // eslint-disable-next-line
                dropdownStyles.dropdownMenuButton,
                styles.diffPaletteDropdown
              )}
            >
              <DiffLegend palette={palette} showMode={showMode} />
            </MenuButton>
          }
          onItemClick={(e) => onChange(e.value)}
        >
          {paletteList.map((p) => (
            <MenuItem key={p.name} value={p}>
              <div>
                <label>{p.name}</label>
                <div className={styles.dropdownItem}>
                  <DiffLegend palette={p} showMode={showMode} />

                  {p === palette ? <CheckIcon /> : null}
                </div>
              </div>
            </MenuItem>
          ))}
        </Dropdown>
      </div>
    </>
  );
};

/**
 * TODO: unify this and toolbar's
 * Custom hook that returns the size ('large' | 'small')
 * that should be displayed
 * based on the toolbar width
 */
// arbitrary value
// as a simple heuristic, try to run the comparison view
// and see when the buttons start to overlap
const WIDTH_THRESHOLD = 13 * 37;
const useSizeMode = (target: React.RefObject<HTMLDivElement>) => {
  const [size, setSize] = React.useState<'large' | 'small'>('large');

  const calcMode = (width: number) => {
    if (width < WIDTH_THRESHOLD) {
      return 'small';
    }
    return 'large';
  };

  React.useLayoutEffect(() => {
    if (target.current) {
      const { width } = target.current.getBoundingClientRect();

      setSize(calcMode(width));
    }
  }, [target.current]);

  useResizeObserver(target, (entry: ResizeObserverEntry) => {
    setSize(calcMode(entry.contentRect.width));
  });

  return size;
};

export default DiffLegendPaletteDropdown;
