import React from 'react';
import Notifications, { store } from '@pyroscope/ui/Notifications';
import Button from '@pyroscope/ui/Button';
import '../sass/profile.scss';

export default {
  title: 'components/Notifications',
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

  const warning = () =>
    store.addNotification({
      title: 'Warning',
      message: 'Warning message',
      type: 'warning',
    });

  const arbitraryJSXElement = () =>
    store.addNotification({
      title: 'Info',
      message: (
        <>
          Info message <a href="">i am a link</a>
        </>
      ),
      type: 'info',
    });

  return (
    <div>
      <Button onClick={() => info()}>Info</Button>
      <Button onClick={() => danger()}>Danger</Button>
      <Button onClick={() => success()}>Success</Button>
      <Button onClick={() => warning()}>Warning</Button>
      <Button onClick={() => arbitraryJSXElement()}>
        Arbitrary JSX Element
      </Button>
      <Notifications />
    </div>
  );
};
