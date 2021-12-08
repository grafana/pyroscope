import React, { useEffect } from 'react';
import ReactNotification, {
  store as libStore,
  ReactNotificationOptions,
  DismissOptions,
} from 'react-notifications-component';
import 'react-notifications-component/dist/theme.css';

export default function Notifications() {
  // render notifications from the server
  // after this component has been initialized
  useEffect(() => {
    // the server is supposed to add this
    // to the index.html
    const { notificationText } = window as any;

    if (notificationText) {
      // TODO
      // distinguish between notification types?
      store.addNotification({
        message: notificationText,
        type: 'danger',
        dismiss: {
          duration: 0,
          showIcon: true,
        },
      });
    }
  }, []);

  return <ReactNotification />;
}

const defaultParams: Partial<ReactNotificationOptions> = {
  insert: 'top',
  container: 'top-right',
  animationIn: ['animate__animated', 'animate__fadeIn'],
  animationOut: ['animate__animated', 'animate__fadeOut'],
};

export type NotificationOptions = {
  title?: string;
  message: string;
  type: 'success' | 'danger' | 'info';

  dismiss?: DismissOptions;
};

export const store = {
  addNotification({ title, message, type, dismiss }: NotificationOptions) {
    dismiss = dismiss || {
      duration: 5000,
      showIcon: true,
    };

    libStore.addNotification({
      ...defaultParams,
      title,
      message,
      type,
      dismiss,
      container: 'top-right',
    });
  },
};
