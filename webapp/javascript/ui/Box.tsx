import React from 'react';
import styles from './Box.module.scss';

/**
 * Box renders its children with a box around it
 */

export interface BoxProps {
  children: React.ReactNode;
  // Disable padding, disabled by default since it should be used for more special cases
  noPadding?: boolean;

  // Additional classnames
  className?: string;
}
export default function Box(props: BoxProps) {
  const { children, noPadding, className = '' } = props;

  const paddingClass = noPadding ? '' : styles.padding;

  return (
    <div className={`${styles.box} ${paddingClass} ${className}`}>
      {children}
    </div>
  );
}
