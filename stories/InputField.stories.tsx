/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import InputField from '@ui/InputField';
import { ComponentStory, ComponentMeta } from '@storybook/react';

const Template: ComponentStory<typeof InputField> = (args) => (
  <InputField type="password" {...args} />
);

export default {
  title: 'Components/InputField',
  component: InputField,
  argTypes: {
    type: {
      options: ['text', 'password'],
      control: { type: 'select' },
    },
  },
} as ComponentMeta<typeof InputField>;

export const Inputfield = Template.bind({});
Inputfield.args = {
  label: 'Sample text',
  placeholder: 'Sample text',
  type: 'text',
};
