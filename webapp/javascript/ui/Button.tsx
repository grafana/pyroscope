import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faAlignLeft } from '@fortawesome/free-solid-svg-icons/faAlignLeft';
import { IconDefinition } from '@fortawesome/fontawesome-common-types';
import styles from './Button.module.scss';

export interface ButtonProps {
  /** Whether the button is disabled or not */
  disabled?: boolean;
  /** Whether the button is disabled or not */
  primary?: boolean;
  icon: IconDefinition;
  children: React.ReactNode;
}

export default function Button({
  disabled = false,
  primary = false,
  icon,
  children,
  ...props
}: ButtonProps) {
  return (
    <button disabled={disabled} className={styles.button}>
      {icon ? <FontAwesomeIcon icon={icon} /> : null}
      {children}
    </button>
  );
}
