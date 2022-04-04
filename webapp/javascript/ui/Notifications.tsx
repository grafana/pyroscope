import React from 'react';
import ReactNotification, {
  store as libStore,
  ReactNotificationOptions,
  DismissOptions,
} from 'react-notifications-component';
import 'react-notifications-component/dist/scss/notification.scss';

export default function Notifications() {
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
  additionalInfo?: string[];
  type: 'success' | 'danger' | 'info';

  dismiss?: DismissOptions;
  onRemoval?: ((id: string, removedBy: ShamefulAny) => void) | undefined;
};

function Message({
  message,
  additionalInfo,
}: {
  message: string;
  additionalInfo?: string[];
}) {
  return (
    <div>
      {message}
      <br />
      <br />
      <small>
        {additionalInfo && 'Additional info:'}

        {additionalInfo &&
          additionalInfo.map((a) => {
            return <div>{a}</div>;
          })}
      </small>
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
      showIcon: true,
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
