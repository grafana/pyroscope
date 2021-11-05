/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Button from '@ui/Button';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';

const Template: ComponentStory<typeof Button> = (args) => (
  <Button {...args}>Button</Button>
);

export default {
  title: 'Pyroscope/Button',
  component: Button,
} as ComponentMeta<typeof Button>;

export const Default = Template.bind({});
Default.args = {
  disabled: false,
};

export const DefaultWithIcon = () => (
  <Button disabled icon={faAlignLeft}>
    Button with icon
  </Button>
);

export const IconOnly = () => <Button icon={faSyncAlt} />;

export const GroupedButtons = () => (
  <>
    <Button grouped>Button</Button>
    <Button grouped>Button 2</Button>
    <Button grouped>Button 3</Button>
  </>
);
