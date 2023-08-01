/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import {
  Tabs as MuiTabs,
  Tab as MuiTab,
  TabsProps,
  TabProps,
} from '@mui/material';
import styles from './Tabs.module.scss';

interface TabPanelProps {
  index: number;
  value: number;
  children: React.ReactNode;
}

export function Tabs({ children, value, onChange }: TabsProps) {
  return (
    <MuiTabs
      TabIndicatorProps={{
        hidden: true, // hide indicator
      }}
      className={styles.tabs}
      value={value}
      onChange={onChange}
    >
      {children}
    </MuiTabs>
  );
}

export function Tab({ label, ...rest }: TabProps) {
  return (
    <MuiTab disableRipple className={styles.tab} {...rest} label={label} />
  );
}

export function TabPanel({ children, value, index, ...other }: TabPanelProps) {
  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`tabpanel-${index}`}
      aria-labelledby={`tab-${index}`}
      {...other}
    >
      {value === index && <div>{children}</div>}
    </div>
  );
}
