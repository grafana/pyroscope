import React, { useState, ReactNode, CSSProperties, RefObject } from 'react';
import clsx from 'clsx';

import styles from './Table.module.scss';

export interface Cell {
  value: ReactNode | string;
  style?: CSSProperties;
  [k: string]: string | undefined | CSSProperties | ReactNode;
}

export interface HeadCell {
  name: string;
  label: string;
  sortable?: number;
  [k: string]: string | number | undefined;
}

export interface BodyRow {
  'data-row'?: string;
  isRowSelected?: boolean;
  isRowDisabled?: boolean;
  cells: Cell[];
  onClick?: () => void;
}

interface Table {
  headRow: HeadCell[];
  // either error or rows should be passed
  bodyRows?: BodyRow[];
  // todo handle error
  // or error or bodyRows strict type
  // we can get
  error?: { value: string; className: string };
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
      // @ts-ignore
      ref={props?.tableBodyRef}
    >
      <thead>
        <tr>
          {props.table.headRow.map(
            ({ sortable, label, name, ...rest }: any, idx: number) =>
              !sortable ? (
                // eslint-disable-next-line react/no-array-index-key
                <th key={idx} {...rest}>
                  {label}
                </th>
              ) : (
                <th
                  {...rest}
                  // eslint-disable-next-line react/no-array-index-key
                  key={idx}
                  className={styles.sortable}
                  onClick={() => props.updateSortParams(name)}
                >
                  {label}
                  <span
                    className={clsx(styles.sortArrow, {
                      [styles[props.sortByDirection]]: props.sortBy === name,
                    })}
                  />
                </th>
              )
          )}
        </tr>
      </thead>
      <tbody>
        {props.table?.error ? (
          <tr className={props.table.error.className}>
            <td colSpan={props.table.headRow.length}>
              {props.table.error.value}
            </td>
          </tr>
        ) : (
          // @ts-ignore
          props.table.bodyRows.map(
            ({ cells, isRowSelected, isRowDisabled, ...rest }) => {
              // The problem is that when you switch apps or time-range and the function
              // names stay the same it leads to an issue where rows don't get re-rendered
              // So we force a rerender each time.
              const renderID = Math.random();

              return (
                <tr
                  key={renderID}
                  {...rest}
                  className={clsx({
                    [styles.isRowSelected]: isRowSelected,
                    // todo: add styles for disabled
                    [styles.isRowDisabled]: isRowDisabled,
                  })}
                >
                  {cells &&
                    cells.map(
                      ({ style, value, ...rest }: Cell, index: number) => (
                        <td key={renderID + index} style={style} {...rest}>
                          {value}
                        </td>
                      )
                    )}
                </tr>
              );
            }
          )
        )}
      </tbody>
    </table>
  );
}

export default Table;
