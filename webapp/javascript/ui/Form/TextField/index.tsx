/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import cx from 'classnames';
import styles from './index.module.scss';

interface TextFieldProps extends React.InputHTMLAttributes<HTMLInputElement> {
  variant?: 'dark' | 'light';
  className?: string;
  label: string;
}

function TextField(
  props: TextFieldProps,
  ref: React.ForwardedRef<HTMLInputElement>
) {
  const { className, label, variant = 'dark' } = props;

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
          {...props}
        />
      </label>
    </div>
  );
}

export default React.forwardRef(TextField);
