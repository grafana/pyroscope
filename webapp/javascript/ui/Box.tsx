import React, { useState } from 'react';
import classNames from 'classnames/bind';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChevronDown } from '@fortawesome/free-solid-svg-icons/faChevronDown';
import styles from './Box.module.scss';

const cx = classNames.bind(styles);
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

export interface CollapseBoxProps {
  /** must be non empty */
  title: string;
  children: React.ReactNode;
}

export function CollapseBox({ title, children }: CollapseBoxProps) {
  const [collapsed, toggleCollapse] = useState(false);

  return (
    <div className={styles.collapseBox}>
      <div
        onClick={() => toggleCollapse((c) => !c)}
        className={styles.collapseTitle}
        aria-hidden
      >
        <div>{title}</div>
        <FontAwesomeIcon
          className={cx({
            [styles.collapseIcon]: true,
            [styles.collapsed]: collapsed,
          })}
          icon={faChevronDown}
        />
      </div>
      <Box
        className={cx({
          [styles.collapsedContent]: collapsed,
        })}
      >
        {children}
      </Box>
    </div>
  );
}
