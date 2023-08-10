import React, { ReactNode } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import type { IconDefinition } from '@fortawesome/fontawesome-common-types';
import cx from 'classnames';
import styles from './Button.module.scss';

export interface ButtonProps {
  kind?: 'default' | 'primary' | 'secondary' | 'danger' | 'outline' | 'float';
  // kind?: 'default' | 'primary' | 'secondary' | 'danger' | 'float' | 'outline';
  /** Whether the button is disabled or not */
  disabled?: boolean;
  icon?: IconDefinition;
  iconNode?: ReactNode;

  children?: React.ReactNode;

  /** Buttons are grouped so that only the first and last have clear limits */
  grouped?: boolean;

  onClick?: (event: React.MouseEvent<HTMLButtonElement>) => void;

  // TODO
  // for the full list use refer to https://developer.mozilla.org/en-US/docs/Web/HTML/Element/input
  type?: 'button' | 'submit';
  ['data-testid']?: string;

  ['aria-label']?: string;

  className?: string;

  id?: string;
  form?: React.ButtonHTMLAttributes<HTMLButtonElement>['form'];

  /** disable a box around it */
  noBox?: boolean;

  /** ONLY use this if within a modal (https://stackoverflow.com/a/71848275 and https://citizensadvice.github.io/react-dialogs/modal/auto_focus/index.html) */
  autoFocus?: React.ButtonHTMLAttributes<HTMLButtonElement>['autoFocus'];
}

const Button = React.forwardRef(function Button(
  {
    disabled = false,
    kind = 'default',
    type = 'button',
    icon,
    iconNode,
    children,
    grouped,
    onClick,
    id,
    className,
    form,
    noBox,
    autoFocus,
    ...props
  }: ButtonProps,
  ref: React.LegacyRef<HTMLButtonElement>
) {
  return (
    <button
      // needed for tooltip
      // eslint-disable-next-line react/jsx-props-no-spreading
      {...props}
      id={id}
      ref={ref}
      type={type}
      data-testid={props['data-testid']}
      disabled={disabled}
      onClick={onClick}
      form={form}
      autoFocus={autoFocus} // eslint-disable-line
      aria-label={props['aria-label']}
      className={cx(
        styles.button,
        grouped ? styles.grouped : '',
        getKindStyles(kind),
        className,
        noBox && styles.noBox,
        iconNode && styles.customIcon,
        !icon && !iconNode && styles.noIcon
      )}
    >
      {icon ? (
        <FontAwesomeIcon
          icon={icon}
          className={children ? styles.iconWithText : ''}
        />
      ) : null}
      {iconNode}
      {children}
    </button>
  );
});

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

    case 'danger': {
      return styles.danger;
    }

    case 'outline': {
      return styles.outline;
    }

    case 'float': {
      return styles.float;
    }

    default: {
      throw Error(`Unsupported kind ${kind}`);
    }
  }
}

export default Button;
