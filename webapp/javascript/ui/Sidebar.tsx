/* eslint-disable react/jsx-props-no-spreading */
import React, { useEffect } from 'react';
import {
  ProSidebar,
  Menu as RProMenu,
  MenuItem as RProMenuItem,
  SubMenu as RProSubMenu,
  SidebarFooter as RProFooter,
  SidebarHeader as RProHeader,
  SidebarContent as RProContent,
  MenuItemProps,
  SubMenuProps,
} from 'react-pro-sidebar';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import styles from './Sidebar.module.css';
import './Sidebar.scss';

export interface SidebarProps {
  children: React.ReactNode[];

  collapsed?: boolean;
  className?: string;
}

// Sidebar is an abstraction over react-pro-sidebar
// So that we can some day reimplement its functinoality ourselves
export default function Sidebar(props: SidebarProps) {
  const { children, collapsed, className } = props;

  return (
    <ProSidebar className={className} collapsed={collapsed}>
      {children}
    </ProSidebar>
  );
}

export function MenuItem(props: MenuItemProps) {
  // wrap the received icon with FontAwesomeIcon
  // to make the API easier to user
  let { icon } = props;
  let { className } = props;
  if (icon) {
    icon = <FontAwesomeIcon icon={props.icon} />;
    className = `${props.className} ${styles.menuWithIcon}`;
  }

  return <RProMenuItem {...props} icon={icon} className={className} />;
}

export function SubMenu(props: SubMenuProps & { active: boolean }) {
  // wrap the received icon with FontAwesomeIcon
  // to make the API easier to user
  let { icon, popperarrow, className } = props;
  const { active } = props;
  if (icon) {
    icon = <FontAwesomeIcon icon={props.icon} />;
  }

  if (popperarrow === undefined) {
    // set arrow between element and menu when collapsed by default, since that makes ux better
    popperarrow = true;
  }

  if (active) {
    if (!className) {
      className = '';
    }

    className += ' active';
  }

  return (
    <RProSubMenu
      {...props}
      icon={icon}
      popperarrow={popperarrow}
      className={className}
    />
  );
}

// Re-export the type so that end users only interact with our abstraction
export const Menu = RProMenu;
export const SidebarHeader = RProHeader;
export const SidebarFooter = RProFooter;
export const SidebarContent = RProContent;
