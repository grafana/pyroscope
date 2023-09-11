import React, { ReactNode } from 'react';
import type { FieldError, Merge, FieldErrorsImpl } from 'react-hook-form';
import styles from './index.module.css';

interface StatusMessageProps {
  message?:
    | string
    | FieldError
    | Merge<FieldError, FieldErrorsImpl<ShamefulAny>>;
}

export default function ValidationError({ message }: StatusMessageProps) {
  if (!message) {
    return null;
  }

  return <div className={styles.validationError}>{message as ReactNode}</div>;
}
