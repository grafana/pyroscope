import React from 'react';
import cx from 'classnames';
import styles from './StatusMessage.module.css';

interface StatusMessageProps {
  type: 'error' | 'success';
  message: string;
}

export default function StatusMessage({ type, message }: StatusMessageProps) {
  const getClassnameForType = () => {
    switch (type) {
      case 'error':
        return styles.error;
      case 'success':
        return styles.success;
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
      {message}
    </div>
  ) : null;
}
