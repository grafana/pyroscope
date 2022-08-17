/* eslint-disable react/jsx-props-no-spreading */
import React, { useState, ReactNode, CSSProperties, RefObject } from 'react';
import clsx from 'clsx';

// eslint-disable-next-line css-modules/no-unused-class
import styles from './Table.module.scss';

interface CustomProp {
  [k: string]: string | CSSProperties | ReactNode | number | undefined;
}

export interface Cell extends CustomProp {
  value: ReactNode | string;
  style?: CSSProperties;
}

interface HeadCell extends CustomProp {
  name: string;
  label: string;
  sortable?: number;
  default?: boolean;
}

export interface BodyRow {
  'data-row'?: string;
  isRowSelected?: boolean;
  isRowDisabled?: boolean;
  cells: Cell[];
  onClick?: () => void;
  className?: string;
}

export type TableBodyType =
  | {
      type: 'not-filled';
      value: string | ReactNode;
      bodyClassName?: string;
    }
  | {
      type: 'filled';
      bodyRows: BodyRow[];
    };

type Table = TableBodyType & {
  headRow: HeadCell[];
};

interface TableSortProps {
  sortBy: string;
  updateSortParams: (v: string) => void;
  sortByDirection: 'desc' | 'asc';
}

export const useTableSort = (headRow: HeadCell[]): TableSortProps => {
  const defaultSortByCell =
    headRow.filter((row) => row?.default)[0] || headRow[0];
  const [sortBy, updateSortBy] = useState(defaultSortByCell.name);
  const [sortByDirection, setSortByDirection] = useState<'desc' | 'asc'>(
    'desc'
  );

  const updateSortParams = (newSortBy: string) => {
    let dir = sortByDirection;

    if (sortBy === newSortBy) {
      dir = dir === 'asc' ? 'desc' : 'asc';
    } else {
      dir = 'desc';
    }

    updateSortBy(newSortBy);
    setSortByDirection(dir);
  };

  return { sortBy, sortByDirection, updateSortParams };
};

interface TableProps {
  sortByDirection?: string;
  sortBy?: string;
  updateSortParams?: (newSortBy: string) => void;
  table: Table;
  tableBodyRef?: RefObject<HTMLTableSectionElement>;
  className?: string;
}

function Table({
  sortByDirection,
  sortBy,
  updateSortParams,
  table,
  tableBodyRef,
  className,
}: TableProps) {
  const hasSort = sortByDirection && sortBy && updateSortParams;
  return (
    <table
      className={clsx(styles.table, {
        [className || '']: className,
      })}
      data-testid="table-ui"
    >
      <thead>
        <tr>
          {table.headRow.map(
            ({ sortable, label, name, ...rest }, idx: number) =>
              !sortable || table.type === 'not-filled' || !hasSort ? (
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
                  onClick={() => updateSortParams(name)}
                >
                  {label}
                  <span
                    className={clsx(styles.sortArrow, {
                      [styles[sortByDirection]]: sortBy === name,
                    })}
                  />
                </th>
              )
          )}
        </tr>
      </thead>
      <tbody ref={tableBodyRef}>
        {table.type === 'not-filled' ? (
          <tr className={table?.bodyClassName}>
            <td colSpan={table.headRow.length}>{table.value}</td>
          </tr>
        ) : (
          table.bodyRows?.map(
            ({ cells, isRowSelected, isRowDisabled, className, ...rest }) => {
              // The problem is that when you switch apps or time-range and the function
              // names stay the same it leads to an issue where rows don't get re-rendered
              // So we force a rerender each time.
              const renderID = Math.random();

              return (
                <tr
                  key={renderID}
                  {...rest}
                  className={clsx(className, {
                    [styles.isRowSelected]: isRowSelected,
                    [styles.isRowDisabled]: isRowDisabled,
                  })}
                >
                  {cells &&
                    cells.map(
                      ({ style, value, ...rest }: Cell, index: number) => (
                        // eslint-disable-next-line react/no-array-index-key
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
