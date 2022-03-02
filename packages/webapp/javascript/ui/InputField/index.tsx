import React, { InputHTMLAttributes } from 'react';

import styles from './InputField.module.css';

interface IInputFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
}

/* eslint-disable react/jsx-props-no-spreading */
function InputField({ label, ...rest }: IInputFieldProps) {
  return (
    <div className={styles.inputWrapper}>
      <h4>{label}</h4>
      <input {...rest} />
    </div>
  );
}

export default InputField;
