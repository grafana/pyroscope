import React from 'react';
import cx from 'classnames';
import styles from './StatusMessage.module.css';

interface StatusMessageProps {
  type: 'error' | 'success';
  message: string;
}

export default function StatusMessage({ type, message }: StatusMessageProps) {
  return message ? (
    <div
      className={cx({
        [styles.statusMessage]: true,
        [styles.error]: type === 'error',
        [styles.success]: type === 'success',
      })}
    >
      {message}
    </div>
  ) : null;
}
