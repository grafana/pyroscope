import React from 'react';
import ShowModal from '@ui/Modals';
import ConfirmDelete from '@ui/Modals/ConfirmDelete';
import Button from '@ui/Button';
import { ComponentStory, ComponentMeta } from '@storybook/react';

const Template: ComponentStory<typeof ConfirmDelete> = ({ object }) => {
  const handleClick = () => {
    ConfirmDelete(object);
  };

  return (
    <Button kind="danger" onClick={handleClick}>
      Delete button text
    </Button>
  );
};

export default {
  title: 'Components/Modals',
  component: ConfirmDelete,
} as ComponentMeta<typeof ConfirmDelete>;

export const ConfirmDeleteModal = Template.bind({});
ConfirmDeleteModal.args = {
  object: 'sample entity',
};

export const ShowConfirmModal = ({ title, confirmButtonText, danger }) => {
  const handleClick = () => {
    ShowModal({ title, confirmButtonText, danger });
  };

  return (
    <Button kind={danger ? 'danger' : undefined} onClick={handleClick}>
      Button text
    </Button>
  );
};

ShowConfirmModal.args = {
  title: 'Sample modal text',
  confirmButtonText: 'Sample button text',
  danger: false,
};
