import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { IconDefinition } from '@fortawesome/fontawesome-common-types';
import styles from './Button.module.scss';

export interface ButtonProps {
  /** Whether the button is disabled or not */
  disabled?: boolean;
  /** Whether the button is disabled or not */
  primary?: boolean;
  icon?: IconDefinition;
  children?: React.ReactNode;

  /** Buttons are grouped so that only the first and last have clear limits */
  grouped?: boolean;
}

export default function Button({
  disabled = false,
  primary = false,
  icon,
  children,
  grouped,
  ...props
}: ButtonProps) {
  return (
    <button
      disabled={disabled}
      className={`${styles.button} ${grouped ? styles.grouped : ''}`}
    >
      {icon ? <FontAwesomeIcon icon={icon} /> : null}
      {children}
    </button>
  );
}
