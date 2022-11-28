/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import {
  Tabs as MuiTabs,
  Tab as MuiTab,
  TabsProps,
  TabProps,
} from '@mui/material';
import styles from './Tabs.module.scss';

const Tabs = ({ children, value, onChange }: TabsProps) => {
  return (
    <MuiTabs className={styles.tabs} value={value} onChange={onChange}>
      {children}
    </MuiTabs>
  );
};

const Tab = ({ label, ...rest }: TabProps) => {
  return <MuiTab className={styles.tab} {...rest} label={label} />;
};

const TabPanel = ({
  visible,
  children,
}: {
  children: React.ReactNode;
  visible: boolean;
}) => {
  return visible ? <div>{children}</div> : null;
};

export { Tabs, Tab, TabPanel };
