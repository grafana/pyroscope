import React from 'react';
import cx from 'classnames';
import styles from './StatusMessage.module.css';

interface StatusMessageProps {
  type: 'error' | 'success';
  children: string;
}

export default function StatusMessage({ type, children }: StatusMessageProps) {
  return children ? (
    <div
      className={cx({
        [styles.statusMessage]: true,
        [styles.error]: type === 'error',
        [styles.success]: type === 'success',
      })}
    >
      {children}
    </div>
  ) : null;
}
