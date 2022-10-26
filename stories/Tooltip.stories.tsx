/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { Tooltip2 } from '../webapp/javascript/ui/Tooltip';
import { TooltipInfoIcon } from '../webapp/javascript/ui/TooltipInfoIcon';
//import Tooltip from '@mui/material/Tooltip';
import { ComponentMeta } from '@storybook/react';
import '../webapp/sass/profile.scss';

export default {
  title: 'Components/Tooltip',
  component: Tooltip2,
} as ComponentMeta<typeof Tooltip2>;

export const MyTooltip = () => {
  return (
    <Tooltip2 title="I should display be displayed on hover">
      <span>hover me</span>
    </Tooltip2>
  );
};

export const TooltipInfo = () => {
  return (
    <Tooltip2 title="use me for informational data">
      <TooltipInfoIcon />
    </Tooltip2>
  );
};
//export const MyTooltip = () => (
//  <Tooltip2 placement="top" title={'My title'}>
//    <span style={{ marginLeft: '100px' }}>my test</span>
//  </Tooltip2>
//);
