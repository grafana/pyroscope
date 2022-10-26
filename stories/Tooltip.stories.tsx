/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { Tooltip } from '../webapp/javascript/ui/Tooltip';
import { TooltipInfoIcon } from '../webapp/javascript/ui/TooltipInfoIcon';
import { ComponentMeta } from '@storybook/react';
import '../webapp/sass/profile.scss';

export default {
  title: 'Components/Tooltip',
  component: Tooltip,
} as ComponentMeta<typeof Tooltip>;

export const MyTooltip = () => {
  return (
    <Tooltip title="I should display be displayed on hover">
      <span>hover me</span>
    </Tooltip>
  );
};

export const TooltipInfo = () => {
  return (
    <Tooltip title="use me for informational data">
      <TooltipInfoIcon />
    </Tooltip>
  );
};
