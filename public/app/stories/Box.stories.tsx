/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Box, { CollapseBox } from '@pyroscope/ui/Box';
import Button from '@pyroscope/ui/Button';
import { ComponentMeta } from '@storybook/react';
import '../sass/profile.scss';

export default {
  title: 'Components/Box',
  component: Box,
} as ComponentMeta<typeof Box>;

// Just a simple example on how to render other components
export const BoxWithButton = () => (
  <Box>
    <Button>I am a button</Button>
  </Box>
);

// No Padding can be used
// which is useful when defining your own padding
export const BoxWithButtonNoPadding = () => (
  <Box noPadding>
    <h1>Hello, world</h1>
  </Box>
);

export const CollapseBoxWithContent = () => (
  <CollapseBox title="Title">
    <h1>Hello, world</h1>
  </CollapseBox>
);

export const CollapseBoxWhenTitleIsEmptyString = () => (
  <CollapseBox title="">
    <h1>Hello, world</h1>
  </CollapseBox>
);
