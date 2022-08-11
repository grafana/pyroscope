import React from 'react';
import cx from 'classnames';
import styles from './Table.module.scss';

interface TableProps {
  children: React.ReactNode;
  className?: string;
}
export default function Table({ children, className }: TableProps) {
  return <table className={cx(styles.table, className)}>{children}</table>;
}
