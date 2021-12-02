/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import {
  ProSidebar,
  Menu,
  MenuItem as RProMenuItem,
  SubMenu as RProSubMenu,
  MenuItemProps,
} from 'react-pro-sidebar';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import styles from './Sidebar.module.css';
import './Sidebar.scss';

export interface SidebarProps {
  children: React.ReactNode[];
}

// Sidebar is an abstraction over react-pro-sidebar
export default function Sidebar(props: SidebarProps) {
  const { children } = props;
  return (
    <ProSidebar>
      <Menu iconShape="square">{children}</Menu>
    </ProSidebar>
  );
}

export function MenuItem(props: MenuItemProps) {
  // wrap the received icon with FontAwesomeIcon
  // to make the API easier to users
  let { icon } = props;
  let { className } = props;
  if (icon) {
    icon = <FontAwesomeIcon icon={props.icon} />;
    className = `${props.className} ${styles.menuWithIcon}`;
  }

  return <RProMenuItem {...props} icon={icon} className={className} />;
}

// Re-export the type so that end users only interact with our abstraction
export const SubMenu = RProSubMenu;
