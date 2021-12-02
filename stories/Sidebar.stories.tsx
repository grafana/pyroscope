/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import Sidebar, { MenuItem, SubMenu } from '@ui/Sidebar';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';
import { faBaby } from '@fortawesome/free-solid-svg-icons/faBaby';

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
      <MenuItem>Component 1</MenuItem>
      {/* <MenuItem icon={<FontAwesomeIcon icon={faClock} />}>Component 2</MenuItem>*/}
      <MenuItem icon={faClock}>Item</MenuItem>
      <MenuItem icon={faBaby}>Item with very very very long name</MenuItem>
      <SubMenu title="Submenu">
        <MenuItem icon={faClock}>Item</MenuItem>
        <MenuItem icon={faBaby}>Item with very very very long name</MenuItem>
      </SubMenu>
    </Sidebar>
  );
};
