import React from 'react';
import { DebounceInput } from 'react-debounce-input';
import styles from './Input.module.scss';

interface InputProps {
  testId?: string;
  className?: string;
  type: 'search';
  name: string;
  placeholder: string;
  minLength: number;
  debounceTimeout: number;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  value: string | number;
}

export default function Input({
  testId,
  className,
  type,
  name,
  placeholder,
  minLength,
  debounceTimeout,
  onChange,
  value,
}: InputProps) {
  return (
    <DebounceInput
      data-testid={testId}
      className={`${styles.input} ${className || ''}`}
      type={type}
      name={name}
      placeholder={placeholder}
      minLength={minLength}
      debounceTimeout={debounceTimeout}
      onChange={onChange}
      value={value}
    />
  );
}
