import React from 'react';
import ReactNotification, {
  store as libStore,
  ReactNotificationOptions,
  DismissOptions,
} from 'react-notifications-component';
import 'react-notifications-component/dist/theme.css';

export default function Notification() {
  return <ReactNotification />;
}

const defaultParams: Partial<ReactNotificationOptions> = {
  insert: 'top',
  container: 'top-right',
  animationIn: ['animate__animated', 'animate__fadeIn'],
  animationOut: ['animate__animated', 'animate__fadeOut'],
};

export type NotificationOptions = {
  title: string;
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
