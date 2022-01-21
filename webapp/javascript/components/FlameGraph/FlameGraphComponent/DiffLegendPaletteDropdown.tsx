import React from 'react';
import Dropdown, { MenuItem, MenuButton, MenuRadioGroup } from '@ui/Dropdown';
import Icon from '@ui/Icon';
import dropdownStyles from '@ui/Dropdown.module.scss';
import cx from 'classnames';
import {
  ColorBlindPalette,
  DefaultPalette,
  FlamegraphPalette,
  DefaultPalette,
  ColorBlindPalette,
} from './colorPalette';
import DiffLegend from './DiffLegend';
import styles from './DiffLegendPaletteDropdown.module.css';

const paletteList = [DefaultPalette, ColorBlindPalette];

export const DiffLegendPaletteDropdown = (props) => {
  const { palette, onChange } = props;
  return (
    <Dropdown
      label="Select a palette"
      menuButton={
        <MenuButton
          className={cx(
            // eslint-disable-next-line
            dropdownStyles.dropdownwMenuButton,
            styles.diffPaletteDropdown
          )}
        >
          <DiffLegend palette={palette} />
        </MenuButton>
      }
    >
      <MenuRadioGroup value={palette} onChange={(e) => onChange(e.value)}>
        {paletteList.map((p) => (
          <MenuItem key={p.name} value={p}>
            <DiffLegend palette={p} />
          </MenuItem>
        ))}
      </MenuRadioGroup>
    </Dropdown>
  );
};

export default DiffLegendPaletteDropdown;
