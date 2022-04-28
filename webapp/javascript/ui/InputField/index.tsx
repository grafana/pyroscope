import React, { InputHTMLAttributes, forwardRef, ForwardedRef } from 'react';

import styles from './InputField.module.css';

interface IInputFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  className?: string;
}

/* eslint-disable react/jsx-props-no-spreading */
function InputField(
  { label, className, ...rest }: IInputFieldProps,
  ref?: ForwardedRef<HTMLInputElement>
) {
  return (
    <div className={`${className || ''} ${styles.inputWrapper}`}>
      <h4>{label}</h4>
      <input {...rest} ref={ref} />
    </div>
  );
}

export default forwardRef(InputField);
