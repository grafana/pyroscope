import React from 'react';
import styles from './Select.module.scss';

interface SelectProps {
  ariaLabel: string;
  className?: string;
  value: string;
  onChange: (e: React.ChangeEvent<HTMLSelectElement>) => void;
  name?: string;
  children: React.DetailedHTMLProps<
    React.OptionHTMLAttributes<HTMLOptionElement>,
    HTMLOptionElement
  >[];
}

export default function Select({
  ariaLabel,
  className,
  value,
  onChange,
  children,
  name,
}: SelectProps) {
  return (
    <select
      name={name}
      aria-label={ariaLabel}
      className={`${styles.select} ${className || ''}`}
      value={value}
      onChange={onChange}
    >
      {children}
    </select>
  );
}
