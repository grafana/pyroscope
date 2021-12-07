/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Notification from '@ui/Notification';
import { ComponentStory, ComponentMeta } from '@storybook/react';

const Template: ComponentStory<typeof Notification> = (args) => (
  <Notification {...args}>Button</Notification>
);

export default {
  title: 'Components/Notification',
  component: Notification,
} as ComponentMeta<typeof Notification>;

export const Default = Template.bind({});
Default.args = {
  disabled: false,
};

export const DefaultNotification = () => (
  <Notification>Button with icon</Notification>
);
