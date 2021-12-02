/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import Sidebar, {
  MenuItem,
  SidebarHeader,
  SidebarFooter,
  SidebarContent,
  SubMenu,
  Menu,
} from '@ui/Sidebar';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';
import { faBaby } from '@fortawesome/free-solid-svg-icons/faBaby';
import { faWindowMaximize } from '@fortawesome/free-regular-svg-icons';
import { faChartBar } from '@fortawesome/free-solid-svg-icons/faChartBar';
import { faColumns } from '@fortawesome/free-solid-svg-icons/faColumns';
import { faFileAlt } from '@fortawesome/free-solid-svg-icons/faFileAlt';
import { faSlack } from '@fortawesome/free-brands-svg-icons/faSlack';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import { faKeyboard } from '@fortawesome/free-solid-svg-icons/faKeyboard';

const Template: ComponentStory<typeof Sidebar> = (args) => (
  <Sidebar {...args} />
);

export default {
  title: 'Components/Sidebar',
  component: Sidebar,
} as ComponentMeta<typeof Sidebar>;

export const Default = (args) => {
  return (
    <Sidebar>
      <Menu iconShape="square">
        <MenuItem>Component 1</MenuItem>
        <MenuItem icon={faClock}>Item</MenuItem>
        <MenuItem icon={faBaby}>Item with very very very long name</MenuItem>
        <SubMenu title="Submenu">
          <MenuItem icon={faClock}>Item</MenuItem>
          <MenuItem icon={faBaby}>Item with very very very long name</MenuItem>
        </SubMenu>
      </Menu>
    </Sidebar>
  );
};

// Trying to mimic our sidebar as much as possible
export const OurSidebar = (args) => {
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
};

// export const OurSidebarCollapsed = (args) => {
//  return (
//    <Sidebar collapsed>
//      <Menu iconShape="square">
//        <SubMenu title="Continuous Profiling" open>
//          <MenuItem active icon={faWindowMaximize}>
//            Single View
//          </MenuItem>
//          <MenuItem icon={faColumns}>Comparison View</MenuItem>
//          <MenuItem icon={faChartBar}>Diff View</MenuItem>
//        </SubMenu>
//        <SubMenu title="Adhoc Profiling">
//          <MenuItem icon={faWindowMaximize}>Single View</MenuItem>
//          <MenuItem icon={faColumns}>Comparison View</MenuItem>
//          <MenuItem icon={faChartBar}>Diff View</MenuItem>
//        </SubMenu>
//      </Menu>
//    </Sidebar>
//  );
// };
//
export const SidebarWithHeaderAndFooter = (args) => {
  return (
    <Sidebar>
      <SidebarHeader>Header</SidebarHeader>
      <SidebarContent />
      <SidebarFooter>
        <Menu iconShape="square">
          <MenuItem>Footer Item 1</MenuItem>
          <MenuItem>Footer Item 2</MenuItem>
          <MenuItem>Footer Item 3</MenuItem>
        </Menu>
      </SidebarFooter>
    </Sidebar>
  );
};
