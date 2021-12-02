import React from 'react';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faSlack } from '@fortawesome/free-brands-svg-icons/faSlack';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import { faKeyboard } from '@fortawesome/free-solid-svg-icons/faKeyboard';
import Sidebar, {
  MenuItem,
  SidebarFooter,
  SidebarContent,
  SubMenu,
  Menu,
} from '@ui/Sidebar';
import styles from './Sidebar.module.css';

export default function Sidebar2() {
  return (
    <Sidebar>
      <SidebarContent>
        <Menu iconShape="square">
          <SubMenu title="Continuous Profiling" open>
            <MenuItem active icon={faWindowMaximize}>
              Single View
            </MenuItem>
            <MenuItem icon={faColumns}>Comparison View</MenuItem>
            <MenuItem icon={faChartBar}>Diff View</MenuItem>
          </SubMenu>
          <SubMenu title="Adhoc Profiling">
            <MenuItem icon={faWindowMaximize}>Single View</MenuItem>
            <MenuItem icon={faColumns}>Comparison View</MenuItem>
            <MenuItem icon={faChartBar}>Diff View</MenuItem>
          </SubMenu>
        </Menu>
      </SidebarContent>
      <SidebarFooter>
        <Menu iconShape="square">
          <MenuItem icon={faFileAlt}>Documentation</MenuItem>
          <MenuItem icon={faSlack}>Slack</MenuItem>
          <MenuItem icon={faGithub}>Github</MenuItem>
          <MenuItem icon={faKeyboard}>Shortcuts</MenuItem>
        </Menu>
      </SidebarFooter>
    </Sidebar>
  );
}
