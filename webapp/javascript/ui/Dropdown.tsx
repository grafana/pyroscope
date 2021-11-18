import React from 'react';
import { Menu, MenuButton } from '@szhsin/react-menu';
import styles from './Dropdown.module.scss';

export interface DropdownProps {
  id?: string;

  /** Whether the button is disabled or not */
  disabled?: boolean;
  ['data-testid']?: string;
  className?: string;

  /** Button text*/
  buttonText?: string;
  children?: JSX.Element[] | JSX.Element;
}

export default function Dropdown({
  id,
  children,
  className,
  disabled,
  buttonText,
  ...props
}: DropdownProps) {
  return (
    <Menu
      id={id}
      className={`${className} ${styles.dropdownMenu}`}
      data-testid={props['data-testid']}
      menuButton={
        <MenuButton
          className={`${styles.dropdownMenuButton}`}
          disabled={disabled}
        >
          {buttonText}
        </MenuButton>
      }
    >
      {children}
    </Menu>
  );
}
