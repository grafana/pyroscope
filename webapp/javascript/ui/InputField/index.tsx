import React, { InputHTMLAttributes, ChangeEvent } from 'react';
import Input from '@webapp/ui/Input';
import styles from './InputField.module.css';

interface IInputFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  className?: string;
  name: string;
  placeholder?: string;
  type: 'text' | 'password' | 'email' | 'number';
  value: string;
  onChange: (e: ChangeEvent<HTMLInputElement>) => void;
  id?: string;
}

function InputField({
  label,
  className,
  name,
  onChange,
  placeholder,
  type,
  value,
  id,
}: IInputFieldProps) {
  return (
    <div className={`${className || ''} ${styles.inputWrapper}`}>
      <h4>{label}</h4>
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
