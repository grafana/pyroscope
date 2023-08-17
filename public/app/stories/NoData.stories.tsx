import React from 'react';
import NoData from '@pyroscope/ui/NoData';
import Box from '@pyroscope/ui/Box';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import '../sass/profile.scss';

const Template: ComponentStory<typeof NoData> = () => {
  return (
    <Box>
      <NoData />
    </Box>
  );
};

export default {
  title: 'Components/NoData',
  component: NoData,
} as ComponentMeta<typeof NoData>;

export const ConfirmationModal = Template.bind({});
