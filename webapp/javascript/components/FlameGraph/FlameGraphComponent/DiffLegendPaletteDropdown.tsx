import React from 'react';
import Dropdown, { MenuItem, MenuButton } from '@ui/Dropdown';
import Icon from '@ui/Icon';
import dropdownStyles from '@ui/Dropdown.module.scss';
import cx from 'classnames';
import { ColorBlindPalette, DefaultPalette } from './colorPalette';
import DiffLegend from './DiffLegend';
import CheckIcon from '../../CheckIcon';
import styles from './DiffLegendPaletteDropdown.module.css';

const paletteList = [DefaultPalette, ColorBlindPalette];

export const DiffLegendPaletteDropdown = (props) => {
  const { palette = DefaultPalette, onChange } = props;
  return (
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
          <DiffLegend palette={palette} />
        </MenuButton>
      }
      onItemClick={(e) => onChange(e.value)}
    >
      {paletteList.map((p) => (
        <MenuItem key={p.name} value={p}>
          <div>
            <label>{p.name}</label>
            <div className={styles.dropdownItem}>
              <DiffLegend palette={p} />

              {p === palette ? <CheckIcon /> : null}
            </div>
          </div>
        </MenuItem>
      ))}
    </Dropdown>
  );
};

export default DiffLegendPaletteDropdown;
