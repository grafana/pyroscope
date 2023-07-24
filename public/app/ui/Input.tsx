import React, { Ref, ChangeEvent } from 'react';
import { DebounceInput } from 'react-debounce-input';
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

/**
 * @deprecated use TextField instead
 */
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
Input.displayName = 'Input';

export default Input;
