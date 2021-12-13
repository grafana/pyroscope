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
import Icon from '@ui/Icon';

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
        <MenuItem icon={<Icon icon={faClock} />}>Item</MenuItem>
        <MenuItem icon={<Icon icon={faBaby} />}>Item</MenuItem>
        <MenuItem icon={<Icon icon={faClock} />}>
          Item with very very very long name
        </MenuItem>
        <SubMenu icon={<Icon icon={faClock} />} title="Submenu">
          <MenuItem icon={<Icon icon={faClock} />}>Item</MenuItem>
          <MenuItem icon={<Icon icon={faBaby} />}>
            Item with very very very long name
          </MenuItem>
        </SubMenu>
      </Menu>
    </Sidebar>
  );
};

export const SidebarWithHeaderAndFooter = (args) => {
  return (
    <Sidebar>
      <SidebarHeader>Header</SidebarHeader>
      <SidebarContent />
      <SidebarFooter>
        <Menu>
          <MenuItem icon={<Icon icon={faClock} />}>Item Footer 1</MenuItem>
          <MenuItem icon={<Icon icon={faClock} />}>Item Footer 2</MenuItem>
          <MenuItem icon={<Icon icon={faClock} />}>Item Footer 3</MenuItem>
        </Menu>
      </SidebarFooter>
    </Sidebar>
  );
};
