import React, { useState } from 'react';
import clsx from 'clsx';

import styles from './Table.module.scss';

export interface Cell {
  // type: 'head' | 'body';
  // sortable?:
}

export interface HeadCell {
  name: string;
  label: string;
  sortable?: boolean;
}

interface Table {
  headRow: HeadCell[];
  bodyRows: Cell[][];
  // noDataSmth block/element/text
  // noData: string;
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

  // give type
  const updateSortParams = (newSortBy: any) => {
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
  table: Table;
  sortByDirection: string;
  sortBy: string;
  // give type
  updateSortParams: any;
}

function Table(props: TableProps) {
  return (
    <table className={styles.table} data-testid="table-ui">
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
                    [props.sortByDirection]: props.sortBy === v.name,
                  })}
                />
              </th>
            )
          )}
        </tr>
      </thead>
      <tbody>
        {props.table.bodyRows.map((row) => {
          return (
            <tr>
              {row.map((cell: any) => (
                <td>{cell}</td>
              ))}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

export default Table;
