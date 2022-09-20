import React, { InputHTMLAttributes, ChangeEvent } from 'react';
import cx from 'classnames';
import Input, { UndebouncedInput, UndebouncedInputProps } from '../Input';
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

// TODO: unify with InputField
interface UncontrolledInputFieldProps
  extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  className?: string;
  name: string;
  placeholder?: string;
  type: 'text' | 'password' | 'email' | 'number';
  inputVariant?: UndebouncedInputProps['variant'];
  readOnly?: UndebouncedInputProps['readOnly'];
  value?: string;
}

function UncontrolledInputField({
  label,
  className,
  name,
  placeholder,
  type,
  inputVariant,
  readOnly,
  value,
}: UncontrolledInputFieldProps) {
  return (
    <div className={cx(className, styles.uncontrolledInputFieldWrapper)}>
      <label>
        {label}
        <UndebouncedInput
          type={type}
          placeholder={placeholder}
          name={name}
          readOnly={readOnly}
          variant={inputVariant}
          value={value}
        />
      </label>
    </div>
  );
}

export { UncontrolledInputField };
export default InputField;
