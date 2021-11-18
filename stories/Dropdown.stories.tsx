/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import Dropdown from '@ui/Dropdown';
import { MenuItem, SubMenu } from '@szhsin/react-menu';

const Template: ComponentStory<typeof Dropdown> = (args) => (
  <Selectable {...args} />
);

export default {
  title: 'Components/Dropdown',
  component: Dropdown,
} as ComponentMeta<typeof Dropdown>;

export const Default = Template.bind({});
Default.args = {
  disabled: false,
};

const Selectable = (args) => {
  const [country, setCountry] = useState(null);
  return (
    <Dropdown
      {...args}
      buttonText={country || 'Select a country'}
      onItemClick={(e) => setCountry(e.value)}
    >
      <SubMenu label="Europe">
        <MenuItem value="Italy">Italy</MenuItem>
        <MenuItem value="Spain">Spain</MenuItem>
        <MenuItem value="France">France</MenuItem>
      </SubMenu>
      <SubMenu label="Asia">
        <MenuItem value="Japan">Japan</MenuItem>
        <MenuItem value="China">China</MenuItem>
      </SubMenu>
    </Dropdown>
  );
};

export const Menu = () => {
  const handleClick = (e) => alert(e.value);
  return (
    <Dropdown buttonText="Menu">
      <SubMenu label="Menu 1">
        <MenuItem onClick={handleClick} value="Item 1.1">
          Item 1.1
        </MenuItem>
        <MenuItem onClick={handleClick} value="Item 1.2">
          Item 1.2
        </MenuItem>
      </SubMenu>
      <SubMenu label="Menu 2">
        <MenuItem onClick={handleClick} value="Item 2.1">
          Item 2.1
        </MenuItem>
        <MenuItem onClick={handleClick} value="Item 1.2">
          Item 2.2
        </MenuItem>
      </SubMenu>
    </Dropdown>
  );
};

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
