import React, { useState } from 'react';
import { ComponentMeta } from '@storybook/react';
import { Dialog, DialogHeader } from '../webapp/javascript/ui/Dialog';
import '../webapp/sass/profile.scss';

export default {
  title: 'Components/Dialog',
  component: Dialog,
} as ComponentMeta<typeof Dialog>;

export function dialog() {
  const [open, setOpen] = useState(false);

  return (
    <>
      <button onClick={() => setOpen(!open)}>toggle modal</button>
      <Dialog
        open={open}
        onClose={() => {
          setOpen(false);
        }}
      >
        <DialogHeader>I am a Header</DialogHeader>
      </Dialog>
    </>
  );
}
