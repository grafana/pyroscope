import React, { useState, ReactNode, CSSProperties, RefObject } from 'react';
import clsx from 'clsx';

import styles from './Table.module.scss';

export interface Cell {
  value: ReactNode;
  style?: CSSProperties;
}

export interface HeadCell {
  name: string;
  label: string;
  sortable?: number;
}

export interface BodyRow {
  'data-row'?: string;
  isRowSelected?: boolean;
  cells?: Cell[];
  onClick?: () => void;
  error?: string | ReactNode;
}

interface Table {
  headRow: HeadCell[];
  bodyRows: BodyRow[];
}

// give type
// sortable cells by default
export const useTable = (
  headRow: HeadCell[]
): {
  sortBy: string;
  updateSortParams: any;
  sortByDirection: 'desc' | 'asc';
} => {
  const [sortBy, updateSortBy] = useState(headRow[0].name);
  const [sortByDirection, setSortByDirection] = useState<'desc' | 'asc'>(
    'desc'
  );

  const updateSortParams = (newSortBy: string) => {
    let dir = sortByDirection;

    if (sortBy === newSortBy) {
      dir = dir === 'asc' ? 'desc' : 'asc';
    } else {
      dir = 'asc';
    }

    updateSortBy(newSortBy);
    setSortByDirection(dir);
  };

  return { sortBy, sortByDirection, updateSortParams };
};

interface TableProps {
  sortByDirection: string;
  sortBy: string;
  updateSortParams: (newSortBy: string) => void;
  table: Table;
  tableBodyRef?: RefObject<HTMLTableSectionElement>;
  className?: string;
  // add is loading for tag explorer table
  isLoading?: boolean;
}

function Table(props: TableProps) {
  return (
    <table
      className={clsx(styles.table, {
        [props.className || '']: props?.className,
      })}
      data-testid="table-ui"
      ref={props?.tableBodyRef}
    >
      <thead>
        <tr>
          {props.table.headRow.map((v: any, idx: number) =>
            !v.sortable ? (
              // eslint-disable-next-line react/no-array-index-key
              <th key={idx}>{v.label}</th>
            ) : (
              <th
                // eslint-disable-next-line react/no-array-index-key
                key={idx}
                className={styles.sortable}
                onClick={() => props.updateSortParams(v.name)}
              >
                {v.label}
                <span
                  className={clsx(styles.sortArrow, {
                    [styles[props.sortByDirection]]: props.sortBy === v.name,
                  })}
                />
              </th>
            )
          )}
        </tr>
      </thead>
      <tbody>
        {props.table.bodyRows.map(({ cells, isRowSelected, ...rest }) => {
          // The problem is that when you switch apps or time-range and the function
          // names stay the same it leads to an issue where rows don't get re-rendered
          // So we force a rerender each time.
          const renderID = Math.random();

          return (
            <tr
              key={renderID}
              {...rest}
              className={clsx({ [styles.isRowSelected]: isRowSelected })}
            >
              {cells &&
                cells.map((cell: Cell, index: number) => (
                  <td key={renderID + index} style={cell?.style}>
                    {cell.value}
                  </td>
                ))}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

export default Table;
