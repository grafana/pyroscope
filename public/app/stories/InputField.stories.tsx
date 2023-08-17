/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import InputField from '@pyroscope/ui/InputField';
import Button from '@pyroscope/ui/Button';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import '../sass/profile.scss';

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

export const AsFormInput = ({ type, label, placeholder }) => {
  return (
    <form>
      <p>Example of component usage in a form</p>
      <InputField type={type} label={label} placeholder={placeholder} />
      <InputField type={type} label={label} placeholder={placeholder} />
      <Button>Submit</Button>
    </form>
  );
};
AsFormInput.args = {
  label: 'Sample text',
  placeholder: 'Sample text',
  type: 'text',
};

export const Inputfield = Template.bind({});
Inputfield.args = {
  label: 'Sample text',
  placeholder: 'Sample text',
  type: 'text',
};
