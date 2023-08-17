/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Button from '@pyroscope/ui/Button';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';
import '../sass/profile.scss';

const Template: ComponentStory<typeof Button> = (args) => (
  <Button {...args}>Button</Button>
);

export default {
  title: 'Components/Button',
  component: Button,
} as ComponentMeta<typeof Button>;

export const Default = Template.bind({});
Default.args = {
  disabled: false,
};

export const DefaultWithIcon = () => (
  <Button icon={faClock}>Button with icon</Button>
);

export const IconOnly = () => <Button icon={faSyncAlt} />;

export const Primary = () => <Button kind="primary">Primary</Button>;
export const Secondary = () => <Button kind="secondary">Secondary</Button>;
export const Floating = () => <Button kind="float">Floating</Button>;

export const GroupedButtons = () => (
  <>
    <Button grouped>Button</Button>
    <Button grouped>Button 2</Button>
    <Button grouped>Button 3</Button>
  </>
);
