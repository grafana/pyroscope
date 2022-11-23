import React from 'react';
import ReactNotification, {
  store as libStore,
  ReactNotificationOptions,
  DismissOptions,
} from 'react-notifications-component';
import 'react-notifications-component/dist/scss/notification.scss';
import styles from './Notifications.module.scss';

export default function Notifications() {
  return (
    <div className={styles.notificationsComponent}>
      <ReactNotification />
    </div>
  );
}

const defaultParams: Partial<ReactNotificationOptions> = {
  insert: 'top',
  container: 'top-right',
  animationIn: ['animate__animated', 'animate__fadeIn'],
  animationOut: ['animate__animated', 'animate__fadeOut'],
};

export type NotificationOptions = {
  title?: string;
  message: string | JSX.Element;
  additionalInfo?: string[];
  type: 'success' | 'danger' | 'info' | 'warning';

  dismiss?: DismissOptions;
  onRemoval?: ((id: string, removedBy: ShamefulAny) => void) | undefined;
};

function Message({
  message,
  additionalInfo,
}: {
  message: string | JSX.Element;
  additionalInfo?: string[];
}) {
  const msg = typeof message === 'string' ? <p>{message}</p> : message;
  return (
    <div>
      {msg}
      {additionalInfo && <h4>Additional Info:</h4>}

      {additionalInfo && (
        <ul>
          {additionalInfo.map((a) => {
            return <li key={a}>{a}</li>;
          })}
        </ul>
      )}
    </div>
  );
}

export const store = {
  addNotification({
    title,
    message,
    type,
    dismiss,
    onRemoval,
    additionalInfo,
  }: NotificationOptions) {
    dismiss = dismiss || {
      duration: 5000,
      pauseOnHover: true,
      click: false,
      touch: false,
      showIcon: true,
      onScreen: true,
    };

    libStore.addNotification({
      ...defaultParams,
      title,
      message: <Message message={message} additionalInfo={additionalInfo} />,
      type,
      dismiss,
      onRemoval,
      container: 'top-right',
    });
  },
};
