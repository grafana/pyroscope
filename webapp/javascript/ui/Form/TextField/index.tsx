/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import cx from 'classnames';
import ValidationError from '../ValidationError';
import styles from './index.module.scss';

interface TextFieldProps extends React.InputHTMLAttributes<HTMLInputElement> {
  /** whether to show a light/dark background */
  variant?: 'dark' | 'light';
  className?: string;
  label: string;
  errorMessage?: React.ComponentProps<typeof ValidationError>['message'];
}

function TextField(
  props: TextFieldProps,
  ref: React.ForwardedRef<HTMLInputElement>
) {
  const {
    className,
    label,
    errorMessage,
    variant = 'dark',
    ...inputProps
  } = props;

  return (
    <div className={cx(className, styles.wrapper)}>
      <label>
        {label}
        <input
          ref={ref}
          className={cx(
            styles.input,
            className,
            variant === 'light' && styles.light
          )}
          {...inputProps}
        />
      </label>
      <ValidationError message={errorMessage} />
    </div>
  );
}

export default React.forwardRef(TextField);
