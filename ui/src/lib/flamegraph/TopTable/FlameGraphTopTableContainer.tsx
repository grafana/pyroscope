import { css } from '@emotion/css';
import { memo, useMemo, useState } from 'react';

import { escapeStringForRegex } from '@grafana/data';

import { Icon, type IconType } from '@components/core/Icon';

import { type FlameGraphDataContainer } from '../FlameGraph/dataTransform';
import { type TableData } from '../types';

type Props = {
  data: FlameGraphDataContainer;
  onSymbolClick: (symbol: string) => void;
  // This is used for highlighting the search button in case there is exact match.
  search?: string;
  // We use these to filter out rows in the table if users is doing text search.
  matchedLabels?: Set<string>;
  sandwichItem?: string;
  onSearch: (str: string) => void;
  onSandwich: (str?: string) => void;
  onTableSort?: (sort: string) => void;
};

type SortColumn = 'Symbol' | 'Self' | 'Total';
type SortDirection = 'asc' | 'desc';
type SortState = { column: SortColumn; direction: SortDirection };

type Row = { symbol: string; self: number; total: number };

const FlameGraphTopTableContainer = memo(
  ({ data, onSymbolClick, search, matchedLabels, onSearch, sandwichItem, onSandwich, onTableSort }: Props) => {

    const rows = useMemo(() => {
      const grouped = buildFilteredTable(data, matchedLabels);
      return Object.entries(grouped).map(([symbol, v]) => ({ symbol, self: v.self ?? 0, total: v.total ?? 0 }));
    }, [data, matchedLabels]);

    const [sort, setSort] = useState<SortState>({ column: 'Self', direction: 'desc' });

    const sortedRows = useMemo(() => {
      const dir = sort.direction === 'asc' ? 1 : -1;
      const copy = rows.slice();
      copy.sort((a, b) => {
        if (sort.column === 'Symbol') return a.symbol.localeCompare(b.symbol) * dir;
        const av = sort.column === 'Self' ? a.self : a.total;
        const bv = sort.column === 'Self' ? b.self : b.total;
        return (av - bv) * dir;
      });
      return copy;
    }, [rows, sort]);

    const handleSort = (column: SortColumn) => {
      const next: SortState =
        sort.column === column
          ? { column, direction: sort.direction === 'desc' ? 'asc' : 'desc' }
          : { column, direction: column === 'Symbol' ? 'asc' : 'desc' };
      setSort(next);
      onTableSort?.(`${next.column}_${next.direction}`);
    };

    return (
      <div className={styles.container} data-testid="topTable">
        <div className={styles.scroll}>
          <table className={styles.table} role="table">
            <thead className={styles.thead}>
              <tr role="row" className={styles.headerRow}>
                <th aria-label="Row actions" className={styles.actionHeader} />
                <SortHeader
                  column="Symbol"
                  active={sort.column === 'Symbol'}
                  direction={sort.direction}
                  align="left"
                  onClick={handleSort}
                  className={styles.symbolHeader}
                />
                <SortHeader
                  column="Self"
                  active={sort.column === 'Self'}
                  direction={sort.direction}
                  align="right"
                  onClick={handleSort}
                  className={styles.numericHeader}
                />
                <SortHeader
                  column="Total"
                  active={sort.column === 'Total'}
                  direction={sort.direction}
                  align="right"
                  onClick={handleSort}
                  className={styles.numericHeader}
                />
              </tr>
            </thead>
            <tbody>
              {sortedRows.map((row) => (
                <TableRow
                  key={row.symbol}
                  data={data}
                  row={row}
                  search={search}
                  sandwichItem={sandwichItem}
                  onSymbolClick={onSymbolClick}
                  onSearch={onSearch}
                  onSandwich={onSandwich}
                />
              ))}
            </tbody>
          </table>
        </div>
      </div>
    );
  }
);

FlameGraphTopTableContainer.displayName = 'FlameGraphTopTableContainer';

function SortHeader({
  column,
  active,
  direction,
  align,
  onClick,
  className,
}: {
  column: SortColumn;
  active: boolean;
  direction: SortDirection;
  align: 'left' | 'right';
  onClick: (column: SortColumn) => void;
  className: string;
}) {
  const label = `Sort by column ${column}${active ? (direction === 'desc' ? ', descending' : ', ascending') : ''}`;
  const indicator = active ? (
    <Icon name={direction === 'desc' ? 'angle-down' : 'angle-up'} size={12} />
  ) : null;
  return (
    <th className={className} aria-sort={active ? (direction === 'desc' ? 'descending' : 'ascending') : 'none'}>
      <button
        type="button"
        className={styles.sortBtn}
        style={{ justifyContent: align === 'right' ? 'flex-end' : 'flex-start' }}
        onClick={() => onClick(column)}
        aria-label={label}
        title={label}
      >
        <span>{column}</span>
        {indicator}
      </button>
    </th>
  );
}

type TableRowProps = {
  data: FlameGraphDataContainer;
  row: Row;
  search?: string;
  sandwichItem?: string;
  onSymbolClick: (symbol: string) => void;
  onSearch: (symbol: string) => void;
  onSandwich: (symbol?: string) => void;
};

function TableRow({ data, row, search, sandwichItem, onSymbolClick, onSearch, onSandwich }: TableRowProps) {
  const isSearched = search === `^${escapeStringForRegex(row.symbol)}$`;
  const isSandwiched = sandwichItem === row.symbol;

  const selfDisp = data.valueDisplayProcessor(row.self);
  const totalDisp = data.valueDisplayProcessor(row.total);

  return (
    <tr role="row" className={styles.row}>
      <td className={styles.actionCell}>
        {/* Visual order matches upstream @grafana/ui <Table>: sandwich first,
            then search. Grafana's source JSX has them reversed but its
            IconButton wrapper renders them this way visually. */}
        <ActionButton
          icon="sandwich"
          active={isSandwiched}
          label={isSandwiched ? 'Remove from sandwich view' : 'Show in sandwich view'}
          onClick={() => onSandwich(isSandwiched ? undefined : row.symbol)}
        />
        <ActionButton
          icon="search"
          active={isSearched}
          label={isSearched ? 'Clear from search' : 'Search for symbol'}
          onClick={() => onSearch(isSearched ? '' : row.symbol)}
        />
      </td>
      <td className={styles.symbolCell}>
        <a
          href=""
          role="link"
          title="Highlight symbol"
          aria-label={row.symbol}
          className={styles.symbolLink}
          onClick={(e) => {
            e.preventDefault();
            onSymbolClick(row.symbol);
          }}
        >
          {row.symbol}
        </a>
      </td>
      <td className={styles.numericCell}>{formatValue(selfDisp)}</td>
      <td className={styles.numericCell}>{formatValue(totalDisp)}</td>
    </tr>
  );
}

function ActionButton({
  icon,
  active,
  label,
  onClick,
}: {
  icon: IconType;
  active: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={styles.actionBtn}
      data-active={active}
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      <Icon name={icon} size={14} />
    </button>
  );
}

function formatValue(disp: { text: string; suffix?: string }) {
  return disp.text + (disp.suffix ?? '');
}

export function buildFilteredTable(data: FlameGraphDataContainer, matchedLabels?: Set<string>) {
  // Group the data by label, we show only one row per label and sum the values
  const filteredTable: { [key: string]: TableData } = Object.create(null);

  // Track call stack to detect recursive calls — recursion would double-count
  // a function's "total" if we summed every nested call, so we attribute total
  // only at the outermost call.
  const callStack: string[] = [];

  for (let i = 0; i < data.data.length; i++) {
    const value = data.getValue(i);
    const self = data.getSelf(i);
    const label = data.getLabel(i);
    const level = data.getLevel(i);

    while (callStack.length > level) {
      callStack.pop();
    }

    const isRecursive = callStack.some((entry) => entry === label);

    if (!matchedLabels || matchedLabels.has(label)) {
      filteredTable[label] = filteredTable[label] || {};
      filteredTable[label].self = filteredTable[label].self ? filteredTable[label].self + self : self;

      if (!isRecursive) {
        filteredTable[label].total = filteredTable[label].total ? filteredTable[label].total + value : value;
      }
    }

    callStack.push(label);
  }

  return filteredTable;
}

const styles = {
  container: css({
    label: 'topTableContainer',
    height: '100%',
    minWidth: 0,
    backgroundColor: 'var(--bg-secondary)',
    overflow: 'hidden',
    padding: '8px',
    display: 'flex',
    flexDirection: 'column',
  }),
  scroll: css({
    label: 'topTableScroll',
    flex: 1,
    overflow: 'auto',
    minWidth: 0,
  }),
  table: css({
    label: 'topTable',
    width: '100%',
    // Action (60) + Self (120) + Total (120) + Symbol (160 minimum) = 460.
    // Below this the Symbol column would collapse to zero with tableLayout:
    // fixed; the parent scroll container will horizontally scroll instead.
    minWidth: 460,
    borderCollapse: 'collapse',
    tableLayout: 'fixed',
    fontSize: 'var(--text-sm)',
  }),
  thead: css({
    label: 'topTableThead',
    position: 'sticky',
    top: 0,
    backgroundColor: 'var(--bg-secondary)',
    zIndex: 1,
  }),
  headerRow: css({
    label: 'topTableHeaderRow',
    borderBottom: '1px solid var(--border-weak)',
  }),
  actionHeader: css({
    width: 60,
    padding: 0,
  }),
  symbolHeader: css({
    textAlign: 'left',
    padding: '4px 8px',
  }),
  numericHeader: css({
    textAlign: 'right',
    padding: '4px 8px',
    width: 120,
  }),
  sortBtn: css({
    label: 'topTableSortBtn',
    display: 'inline-flex',
    alignItems: 'center',
    gap: '4px',
    width: '100%',
    background: 'transparent',
    border: 'none',
    color: 'var(--text-secondary)',
    cursor: 'pointer',
    fontWeight: 'var(--weight-medium)',
    padding: 0,
    '&:hover': {
      color: 'var(--text-primary)',
    },
  }),
  row: css({
    label: 'topTableRow',
    borderBottom: '1px solid var(--border-weak)',
    '&:hover': {
      backgroundColor: 'var(--action-hover)',
    },
  }),
  actionCell: css({
    width: 60,
    padding: '2px 4px',
    display: 'flex',
    gap: 2,
  }),
  actionBtn: css({
    label: 'topTableActionBtn',
    width: 24,
    height: 24,
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'transparent',
    border: 'none',
    color: 'var(--text-secondary)',
    borderRadius: 'var(--radius-md)',
    cursor: 'pointer',
    padding: 0,
    '&:hover': {
      color: 'var(--text-primary)',
      backgroundColor: 'var(--action-hover)',
    },
    "&[data-active='true']": {
      color: 'var(--color-primary-text)',
    },
  }),
  symbolCell: css({
    padding: '4px 8px',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  }),
  symbolLink: css({
    color: 'var(--text-link)',
    textDecoration: 'none',
    '&:hover': {
      textDecoration: 'underline',
    },
  }),
  numericCell: css({
    textAlign: 'right',
    padding: '4px 8px',
    fontVariantNumeric: 'tabular-nums',
    color: 'var(--text-primary)',
    width: 120,
  }),
};

export default FlameGraphTopTableContainer;
