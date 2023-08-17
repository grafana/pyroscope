/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import StatusMessage from '@pyroscope/ui/StatusMessage';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import '../sass/profile.scss';

const Template: ComponentStory<typeof StatusMessage> = (args) => (
  <StatusMessage {...args} />
);

export default {
  title: 'Components/StatusMessage',
  component: StatusMessage,
} as ComponentMeta<typeof StatusMessage>;

export const Statusmessage = Template.bind({});
Statusmessage.args = {
  type: 'success',
  message: 'Example message',
};
