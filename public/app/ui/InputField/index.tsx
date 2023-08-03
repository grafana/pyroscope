import React, { InputHTMLAttributes, ChangeEvent } from 'react';
import Input from '../Input';
import styles from './InputField.module.css';

interface InputFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  className?: string;
  name: string;
  placeholder?: string;
  type: 'text' | 'password' | 'email' | 'number';
  value: string;
  onChange: (e: ChangeEvent<HTMLInputElement>) => void;
  id?: string;
}

/**
 * @deprecated use TextField instead
 */
function InputField({
  label,
  className,
  name,
  onChange,
  placeholder,
  type,
  value,
  id,
}: InputFieldProps) {
  return (
    <div className={`${className || ''} ${styles.inputWrapper}`}>
      <label className={styles.label}>{label}</label>
      <Input
        type={type}
        placeholder={placeholder}
        name={name}
        onChange={onChange}
        value={value}
        htmlId={id}
      />
    </div>
  );
}

export default InputField;
