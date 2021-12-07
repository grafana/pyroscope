/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Notification, { store } from '@ui/Notification';
import Button from '@ui/Button';

export default {
  title: 'Notifications',
};

export const notifications = () => {
  const info = () =>
    store.addNotification({
      title: 'Info',
      message: 'Info message',
      type: 'info',
    });

  const danger = () =>
    store.addNotification({
      title: 'Danger',
      message: 'Danger message',
      type: 'danger',
    });

  const success = () =>
    store.addNotification({
      title: 'Success',
      message: 'Success message',
      type: 'success',
    });

  return (
    <div>
      <Button onClick={() => info()}>Info</Button>
      <Button onClick={() => danger()}>Danger</Button>
      <Button onClick={() => success()}>Success</Button>
      <Notification />
    </div>
  );
};
