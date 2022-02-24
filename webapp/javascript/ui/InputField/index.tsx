import React, { InputHTMLAttributes } from 'react';
import cx from 'classnames';

import styles from './InputField.module.css';

interface IInputFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  styling?: string;
}

/* eslint-disable react/jsx-props-no-spreading */
function InputField({ label, styling, ...rest }: IInputFieldProps) {
  return (
    <div
      className={cx({
        [styles.inputWrapper]: true,
        [styling]: true,
      })}
    >
      <h4>{label}</h4>
      <input {...rest} />
    </div>
  );
}

export default InputField;
