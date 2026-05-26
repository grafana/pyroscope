import { css } from '@emotion/css';
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';

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

// Rows are uniform — measured from a default-styled <tr>. Adjusting this is
// a one-character change but the virtualizer math depends on it being exact.
const ROW_HEIGHT = 25;
const OVERSCAN_ROWS = 8;
const HEADER_HEIGHT = 27;

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

    const handleSort = useCallback(
      (column: SortColumn) => {
        setSort((prev) => {
          const next: SortState =
            prev.column === column
              ? { column, direction: prev.direction === 'desc' ? 'asc' : 'desc' }
              : { column, direction: column === 'Symbol' ? 'asc' : 'desc' };
          onTableSort?.(`${next.column}_${next.direction}`);
          return next;
        });
      },
      [onTableSort]
    );

    // Virtualization: track scroll offset + viewport height of the scroll
    // container so we only render visible rows. Without this, the ~1k rows in
    // a real profile cause a noticeable jank when switching to Top Table.
    const scrollRef = useRef<HTMLDivElement>(null);
    const [scrollTop, setScrollTop] = useState(0);
    const [viewportH, setViewportH] = useState(600);
    useEffect(() => {
      const el = scrollRef.current;
      if (!el) return;
      const onScroll = () => setScrollTop(el.scrollTop);
      el.addEventListener('scroll', onScroll, { passive: true });
      const ro = new ResizeObserver((entries) => {
        for (const entry of entries) setViewportH(entry.contentRect.height);
      });
      ro.observe(el);
      setViewportH(el.clientHeight);
      return () => {
        el.removeEventListener('scroll', onScroll);
        ro.disconnect();
      };
    }, []);

    // Subtract HEADER_HEIGHT from the visible window because the sticky thead
    // covers that many pixels at the top of the scroll container.
    const visibleBodyH = Math.max(0, viewportH - HEADER_HEIGHT);
    const firstVisible = Math.max(0, Math.floor((scrollTop - HEADER_HEIGHT) / ROW_HEIGHT));
    const lastVisible = Math.min(
      sortedRows.length,
      Math.ceil((scrollTop + visibleBodyH) / ROW_HEIGHT) + 1
    );
    const startIdx = Math.max(0, firstVisible - OVERSCAN_ROWS);
    const endIdx = Math.min(sortedRows.length, lastVisible + OVERSCAN_ROWS);
    const padTop = startIdx * ROW_HEIGHT;
    const padBottom = (sortedRows.length - endIdx) * ROW_HEIGHT;

    return (
      <div className={styles.container} data-testid="topTable">
        <div ref={scrollRef} className={styles.scroll}>
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
              {padTop > 0 && (
                <tr aria-hidden="true" style={{ height: padTop }}>
                  <td colSpan={4} />
                </tr>
              )}
              {sortedRows.slice(startIdx, endIdx).map((row) => (
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
              {padBottom > 0 && (
                <tr aria-hidden="true" style={{ height: padBottom }}>
                  <td colSpan={4} />
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    );
  }
);

FlameGraphTopTableContainer.displayName = 'FlameGraphTopTableContainer';

function SortHeaderInner({
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
const SortHeader = memo(SortHeaderInner);

type TableRowProps = {
  data: FlameGraphDataContainer;
  row: Row;
  search?: string;
  sandwichItem?: string;
  onSymbolClick: (symbol: string) => void;
  onSearch: (symbol: string) => void;
  onSandwich: (symbol?: string) => void;
};

function TableRowInner({ data, row, search, sandwichItem, onSymbolClick, onSearch, onSandwich }: TableRowProps) {
  const isSearched = search === `^${escapeStringForRegex(row.symbol)}$`;
  const isSandwiched = sandwichItem === row.symbol;

  const selfDisp = data.valueDisplayProcessor(row.self);
  const totalDisp = data.valueDisplayProcessor(row.total);

  const onSandwichClick = useCallback(
    () => onSandwich(isSandwiched ? undefined : row.symbol),
    [onSandwich, isSandwiched, row.symbol]
  );
  const onSearchClick = useCallback(
    () => onSearch(isSearched ? '' : row.symbol),
    [onSearch, isSearched, row.symbol]
  );
  const onLinkClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      onSymbolClick(row.symbol);
    },
    [onSymbolClick, row.symbol]
  );

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
          onClick={onSandwichClick}
        />
        <ActionButton
          icon="search"
          active={isSearched}
          label={isSearched ? 'Clear from search' : 'Search for symbol'}
          onClick={onSearchClick}
        />
      </td>
      <td className={styles.symbolCell}>
        <a
          href=""
          role="link"
          title="Highlight symbol"
          aria-label={row.symbol}
          className={styles.symbolLink}
          onClick={onLinkClick}
        >
          {row.symbol}
        </a>
      </td>
      <td className={styles.numericCell}>{formatValue(selfDisp)}</td>
      <td className={styles.numericCell}>{formatValue(totalDisp)}</td>
    </tr>
  );
}
const TableRow = memo(TableRowInner);

function ActionButtonInner({
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
const ActionButton = memo(ActionButtonInner);

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
    height: ROW_HEIGHT,
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
    width: 20,
    height: 20,
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
