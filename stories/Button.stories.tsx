/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Button from '@ui/Button';
import { ComponentStory, ComponentMeta } from '@storybook/react';

const Template: ComponentStory<typeof Button> = (args) => <Button {...args} />;

// More on default export: https://storybook.js.org/docs/react/writing-stories/introduction#default-export
export default {
  title: 'Example/Button',
  component: Button,
  // More on argTypes: https://storybook.js.org/docs/react/api/argtypes
  //  argTypes: {
  //    backgroundColor: { control: 'color' },
  //  },
} as ComponentMeta<typeof Button>;

export const Primary = Template.bind({});
// More on args: https://storybook.js.org/docs/react/writing-stories/args
Primary.args = {
  primary: true,
  label: 'Button',
};
