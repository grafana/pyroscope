import React from 'react';
import ShowModal from '@pyroscope/ui/Modals';
import Button from '@pyroscope/ui/Button';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import '../sass/profile.scss';

const Template: ComponentStory<typeof ShowModal> = ({
  title,
  confirmButtonText,
  danger,
}) => {
  const handleClick = () => {
    ShowModal({ title, confirmButtonText, danger });
  };

  return (
    <Button kind={danger ? 'danger' : undefined} onClick={handleClick}>
      Button text
    </Button>
  );
};

export default {
  title: 'Components/Modals',
  component: ShowModal,
} as ComponentMeta<typeof ShowModal>;

export const ConfirmationModal = Template.bind({});

ConfirmationModal.args = {
  title: 'Sample modal text',
  confirmButtonText: 'Sample button text',
  danger: false,
};
