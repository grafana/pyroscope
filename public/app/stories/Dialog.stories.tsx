import React, { useState } from 'react';
import { ComponentMeta } from '@storybook/react';
import {
  Dialog,
  DialogFooter,
  DialogHeader,
  DialogBody,
  DialogActions,
} from '@pyroscope/ui/Dialog';
import Button from '@pyroscope/ui/Button';
import '../sass/profile.scss';

export default {
  title: 'Components/Dialog',
  component: Dialog,
} as ComponentMeta<typeof Dialog>;

export function dialog() {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button onClick={() => setOpen(!open)}>Open Modal</Button>
      <Dialog
        aria-labelledby="dialog-header"
        open={open}
        onClose={() => {
          setOpen(false);
        }}
      >
        <>
          <DialogHeader closeable onClose={() => setOpen(false)}>
            <h3 id="dialog-header">I am the Header</h3>
          </DialogHeader>
          <DialogBody>
            <p>I am the body</p>
            <p>
              Phasellus at tellus iaculis nunc ornare porttitor vel at dolor.
              Donec ornare diam sit amet eros posuere, quis vestibulum nunc
              tempus. Vestibulum ante ipsum primis in faucibus orci luctus et
              ultrices posuere cubilia curae; Etiam ullamcorper luctus gravida.
              Quisque vitae euismod diam. Maecenas vulputate et massa hendrerit
              dignissim. Donec consequat nisi eu nisl laoreet tincidunt. Nullam
              dignissim ornare efficitur. Suspendisse at mollis dolor.
              Suspendisse luctus tellus ut metus pretium, sed blandit elit
              sagittis. Praesent arcu urna, consequat vel vehicula mattis,
              viverra nec erat. Vestibulum mattis vehicula arcu, quis iaculis
              dui elementum quis. In in massa tortor. Nullam volutpat nunc
              sapien, vel fringilla risus porta at.
            </p>
          </DialogBody>
          <DialogFooter>
            <DialogActions>
              <Button onClick={() => setOpen(false)}>Cancel</Button>
              <Button
                autoFocus
                onClick={() => {
                  setOpen(false);
                }}
                kind="secondary"
              >
                Ok
              </Button>
            </DialogActions>
          </DialogFooter>
        </>
      </Dialog>
    </>
  );
}
