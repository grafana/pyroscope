import React, { Ref, ChangeEvent } from 'react';
import { DebounceInput } from 'react-debounce-input';
import cx from 'classnames';
import styles from './Input.module.scss';

interface InputProps {
  testId?: string;
  className?: string;
  type: 'search' | 'text' | 'password' | 'email' | 'number';
  name: string;
  placeholder?: string;
  minLength?: number;
  debounceTimeout?: number;
  onChange: (e: ChangeEvent<HTMLInputElement>) => void;
  value: string | number;
  htmlId?: string;
}

const Input = React.forwardRef(
  (
    {
      testId,
      className,
      type,
      name,
      placeholder,
      minLength = 0,
      debounceTimeout = 100,
      onChange,
      value,
      htmlId,
    }: InputProps,
    ref?: Ref<HTMLInputElement>
  ) => {
    return (
      <DebounceInput
        inputRef={ref}
        data-testid={testId}
        className={`${styles.input} ${className || ''}`}
        type={type}
        name={name}
        placeholder={placeholder}
        minLength={minLength}
        debounceTimeout={debounceTimeout}
        onChange={onChange}
        value={value}
        id={htmlId}
      />
    );
  }
);

export default Input;

export interface UndebouncedInputProps {
  className?: string;
  type: 'search' | 'text' | 'password' | 'email' | 'number';
  name: string;
  placeholder?: string;
  onChange?: (e: ChangeEvent<HTMLInputElement>) => void;
  value?: string | number;
  htmlId?: string;
  variant?: 'dark' | 'light';
}
function UndebouncedInput({
  className,
  type,
  name,
  placeholder,
  onChange,
  value,
  htmlId,
  variant = 'dark',
}: UndebouncedInputProps) {
  return (
    <input
      className={cx(
        styles.input,
        className,
        variant === 'light' && styles.light
      )}
      type={type}
      name={name}
      placeholder={placeholder}
      onChange={onChange}
      value={value}
      id={htmlId}
    />
  );
}

export { UndebouncedInput };
