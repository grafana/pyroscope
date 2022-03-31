/* eslint-disable import/prefer-default-export */
import { createAsyncThunk } from '@reduxjs/toolkit';
import { store } from '@webapp/ui/Notifications';
import type { NotificationOptions } from '@webapp/ui/Notifications';

export const addNotification = createAsyncThunk(
  'notifications/add',
  async (opts: NotificationOptions) => {
    return new Promise((resolve) => {
      // TODO:
      // we can at some point add default buttons OK and Cancel
      // which would resolve/reject the promise
      store.addNotification({
        ...opts,
        onRemoval: () => {
          // TODO: fix type
          resolve(null as ShamefulAny);
        },
      });
    });
  }
);

// TODO
// create a store with maintains the notification history?
