import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { IconDefinition } from '@fortawesome/fontawesome-common-types';
import styles from './Button.module.scss';

export interface ButtonProps {
  kind?: 'default' | 'primary' | 'secondary';
  /** Whether the button is disabled or not */
  disabled?: boolean;
  icon?: IconDefinition;
  children?: React.ReactNode;

  /** Buttons are grouped so that only the first and last have clear limits */
  grouped?: boolean;
}

export default function Button({
  disabled = false,
  kind = 'default',
  icon,
  children,
  grouped,
  ...props
}: ButtonProps) {
  return (
    <button
      disabled={disabled}
      className={`${styles.button} ${
        grouped ? styles.grouped : ''
      } ${getKindStyles(kind)}`}
    >
      {icon ? <FontAwesomeIcon icon={icon} className={styles.icon} /> : null}
      {children}
    </button>
  );
}

function getKindStyles(kind: ButtonProps['kind']) {
  switch (kind) {
    case 'default': {
      return styles.default;
    }

    case 'primary': {
      return styles.primary;
    }

    case 'secondary': {
      return styles.secondary;
    }

    default: {
      throw Error(`Unsupported kind ${kind}`);
    }
  }
}
