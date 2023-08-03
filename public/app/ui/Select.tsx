import React, { ReactNode } from 'react';
import styles from './Select.module.scss';

interface SelectProps {
  ariaLabel: string;
  className?: string;
  value: string;
  onChange: (e: React.ChangeEvent<HTMLSelectElement>) => void;
  name?: string;
  children: Array<
    React.DetailedHTMLProps<
      React.OptionHTMLAttributes<HTMLOptionElement>,
      HTMLOptionElement
    >
  >;
  id?: string;
  disabled?: boolean;
}

export default function Select({
  ariaLabel,
  className,
  value,
  onChange,
  children,
  name,
  id,
  disabled,
}: SelectProps) {
  return (
    <select
      id={id}
      disabled={disabled || false}
      name={name}
      aria-label={ariaLabel}
      className={`${styles.select} ${className || ''}`}
      value={value}
      onChange={onChange}
    >
      {children as ReactNode}
    </select>
  );
}
