/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import Dropdown from '@ui/Dropdown';
import Button from '@ui/Button';
import { MenuButton, MenuItem, SubMenu } from '@szhsin/react-menu';

const Template: ComponentStory<any> = (args) => (
  <Dropdown buttonText="Dropdown" theming="dark" {...args}>
    <MenuItem>Option</MenuItem>
  </Dropdown>
);

export default {
  title: 'Components/Dropdown',
  component: Dropdown,
} as ComponentMeta<any>;

export const Default = Template.bind({});
Default.args = {
  disabled: false,
};

export const Menu = () => (
  <Dropdown buttonText="Menu">
    <SubMenu label="Menu 1">
      <MenuItem>Item 1.1</MenuItem>
      <MenuItem>Item 1.2</MenuItem>
    </SubMenu>
    <SubMenu label="Menu 2">
      <MenuItem>Item 1.1</MenuItem>
      <MenuItem>Item 1.2</MenuItem>
    </SubMenu>
  </Dropdown>
);

export const NestedMenu = () => (
  <Dropdown buttonText="Menu">
    <SubMenu label="Menu 1">
      <MenuItem>Item 1.1</MenuItem>
      <SubMenu label="Item 1.2">
        <MenuItem>Item 1.2.1</MenuItem>
        <MenuItem>Item 1.2.2</MenuItem>
      </SubMenu>
    </SubMenu>
  </Dropdown>
);
