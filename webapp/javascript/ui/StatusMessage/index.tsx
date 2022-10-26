import React from 'react';
import cx from 'classnames';
import styles from './StatusMessage.module.scss';

interface StatusMessageProps {
  type: 'error' | 'success' | 'warning';
  message: string;
  action?: React.ReactNode;
}

export default function StatusMessage({
  type,
  message,
  action,
}: StatusMessageProps) {
  const getClassnameForType = () => {
    switch (type) {
      case 'error':
        return styles.error;
      case 'success':
        return styles.success;
      case 'warning':
        return styles.warning;
      default:
        return styles.error;
    }
  };

  return message ? (
    <div
      className={cx({
        [styles.statusMessage]: true,
        [getClassnameForType()]: true,
      })}
    >
      <div>{message}</div>
      <div className={styles.action}>{action}</div>
    </div>
  ) : null;
}
