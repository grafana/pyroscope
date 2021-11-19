/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import Dropdown from '@ui/Dropdown';
import { MenuHeader, MenuItem, SubMenu } from '@szhsin/react-menu';

const Template: ComponentStory<typeof Dropdown> = (args) => (
  <DropdownSelect {...args} />
);

export default {
  title: 'Components/Dropdown',
  component: Dropdown,
} as ComponentMeta<typeof Dropdown>;

export const Default = Template.bind({});
Default.args = {
  disabled: false,
};

const DropdownSelect = (args) => {
  const [country, setCountry] = useState(null);
  return (
    <Dropdown
      {...args}
      label="Select a country"
      value={country}
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
