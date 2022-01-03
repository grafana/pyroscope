import React from 'react';
import {
  ClickEvent,
  Menu,
  MenuHeader,
  SubMenu as LibSubmenu,
  MenuItem as LibMenuItem,
  MenuButton as LibMenuButton,
  FocusableItem as LibFocusableItem,
} from '@szhsin/react-menu';
import styles from './Dropdown.module.scss';

export interface DropdownProps {
  id?: string;

  /** Whether the button is disabled or not */
  disabled?: boolean;
  ['data-testid']?: string;
  className?: string;

  /** Dropdown label */
  label: string;

  /** Dropdown value*/
  value?: string;

  children?: JSX.Element[] | JSX.Element;

  /** Event that fires when an item is activated*/
  onItemClick?: (event: ClickEvent) => void;
}

export default function Dropdown({
  id,
  children,
  className,
  disabled,
  value,
  label,
  onItemClick,
  ...props
}: DropdownProps) {
  return (
    <Menu
      id={id}
      className={`${className} ${styles.dropdownMenu}`}
      data-testid={props['data-testid']}
      onItemClick={onItemClick}
      menuButton={
        <MenuButton
          className={`${styles.dropdownMenuButton}`}
          disabled={disabled}
        >
          {value || label}
        </MenuButton>
      }
    >
      <MenuHeader>{label}</MenuHeader>
      {children}
    </Menu>
  );
}

export const SubMenu = LibSubmenu;
export const MenuItem = LibMenuItem;
export const MenuButton = LibMenuButton;
export const FocusableItem = LibFocusableItem;
