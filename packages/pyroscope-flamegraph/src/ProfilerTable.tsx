import React, { useRef, RefObject, CSSProperties } from 'react';
import type Color from 'color';
import type { Maybe } from 'true-myth';
import { doubleFF, singleFF, Flamebearer } from '@pyroscope/models/src';
import TableUI, {
  useTableSort,
  BodyRow,
  TableBodyType,
} from '@webapp/ui/Table';
import TableTooltip from './Tooltip/TableTooltip';
import { getFormatter } from './format/format';
import {
  colorBasedOnPackageName,
  defaultColor,
  getPackageNameFromStackTrace,
} from './FlameGraph/FlameGraphComponent/color';
import { fitIntoTableCell, FitModes } from './fitMode/fitMode';
import { isMatch } from './search';
import type { FlamegraphPalette } from './FlameGraph/FlameGraphComponent/colorPalette';

const zero = (v?: number) => v || 0;

interface SingleCell {
  type: 'single';
  self: number;
  total: number;
}

interface DoubleCell {
  type: 'double';
  self: number;
  total: number;
  selfLeft: number;
  selfRght: number;
  selfDiff: number;
  totalLeft: number;
  totalRght: number;
  totalDiff: number;
}
type DoubleSortByKeys = keyof DoubleCell & 'name';

function generateCellSingle(
  ff: typeof singleFF,
  cell: SingleCell,
  level: number[],
  j: number
) {
  const c = cell;

  c.type = 'single';
  c.self = zero(c.self) + ff.getBarSelf(level, j);
  c.total = zero(c.total) + ff.getBarTotal(level, j);
  return c;
}

function generateCellDouble(
  ff: typeof doubleFF,
  cell: DoubleCell,
  level: number[],
  j: number
) {
  const c = cell;

  c.type = 'double';
  c.self = zero(c.self) + ff.getBarSelf(level, j);
  c.total = zero(c.total) + ff.getBarTotal(level, j);
  c.selfLeft = zero(c.selfLeft) + ff.getBarSelfLeft(level, j);
  c.selfRght = zero(c.selfRght) + ff.getBarSelfRght(level, j);
  c.selfDiff = zero(c.selfDiff) + ff.getBarSelfDiff(level, j);
  c.totalLeft = zero(c.totalLeft) + ff.getBarTotalLeft(level, j);
  c.totalRght = zero(c.totalRght) + ff.getBarTotalRght(level, j);
  c.totalDiff = zero(c.totalDiff) + ff.getBarTotalDiff(level, j);
  return c;
}

// generates a table from data in flamebearer format
function generateTable(
  flamebearer: Flamebearer
): ((SingleCell | DoubleCell) & { name: string })[] {
  const table: ((SingleCell | DoubleCell) & { name: string })[] = [];
  if (!flamebearer) {
    return table;
  }
  const { names, levels, format } = flamebearer;
  const ff = format !== 'double' ? singleFF : doubleFF;

  const hash: Record<string, (DoubleCell | SingleCell) & { name: string }> = {};
  // eslint-disable-next-line no-plusplus
  for (let i = 0; i < levels.length; i++) {
    const level = levels[i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const key = ff.getBarName(level, j);
      const name = names[key];
      hash[name] = hash[name] || { name: name || '<empty>' };

      // TODO(eh-am): not the most optimal performance wise
      // but better for type checking
      if (format === 'single') {
        generateCellSingle(singleFF, hash[name] as SingleCell, level, j);
      } else {
        generateCellDouble(doubleFF, hash[name] as DoubleCell, level, j);
      }
    }
  }

  return Object.values(hash);
}

// the value must be negative or zero
function neg(v: number) {
  return Math.min(0, v);
}

function backgroundImageStyle(a: number, b: number, color: Color) {
  const w = 148;
  const k = w - (a / b) * w;
  const clr = color.alpha(1.0);
  return {
    backgroundImage: `linear-gradient(${clr}, ${clr})`,
    backgroundPosition: `-${k}px 0px`,
    backgroundRepeat: 'no-repeat',
  };
}

// side: _ | L | R : indicates how to render the diff color
// - _: render both diff color
// - L: only render diff color on the left, if the left is longer than the right (better, green)
// - R: only render diff color on the right, if the right is longer than the left (worse, red)
function backgroundImageDiffStyle(
  palette: FlamegraphPalette,
  a: number,
  b: number,
  total: number,
  color: Color,
  side?: 'L' | 'R'
): React.CSSProperties {
  const w = 148;
  const k = w - (Math.min(a, b) / total) * w;
  const kd = w - (Math.max(a, b) / total) * w;
  const clr = color.alpha(1.0);
  const cld =
    b < a ? palette.goodColor.alpha(0.8) : palette.badColor.alpha(0.8);

  if (side === 'L' && a < b) {
    return {
      backgroundImage: `linear-gradient(${clr}, ${clr})`,
      backgroundPosition: `${neg(-k)}px 0px`,
      backgroundRepeat: 'no-repeat',
    };
  }
  if (side === 'R' && b < a) {
    return {
      backgroundImage: `linear-gradient(${clr}, ${clr})`,
      backgroundPosition: `${neg(-k)}px 0px`,
      backgroundRepeat: 'no-repeat',
    };
  }

  return {
    backgroundImage: `linear-gradient(${clr}, ${clr}), linear-gradient(${cld}, ${cld})`,
    backgroundPosition: `${neg(-k)}px 0px, ${neg(-kd)}px 0px`,
    backgroundRepeat: 'no-repeat',
  };
}

const tableFormatSingle: {
  sortable: number;
  name: 'name' | 'self' | 'total';
  label: string;
}[] = [
  { sortable: 1, name: 'name', label: 'Location' },
  { sortable: 1, name: 'self', label: 'Self' },
  { sortable: 1, name: 'total', label: 'Total' },
];

const tableFormatDiffDef: Record<
  string,
  {
    sortable: number;
    name:
      | 'name'
      | 'selfLeft'
      | 'selfRght'
      | 'selfDiff'
      | 'totalLeft'
      | 'totalRght'
      | 'totalDiff';
    label: string;
  }
> = {
  name: { sortable: 1, name: 'name', label: 'Location' },
  selfLeft: { sortable: 1, name: 'selfLeft', label: 'Self (Left)' },
  selfRght: { sortable: 1, name: 'selfRght', label: 'Self (Right)' },
  selfDiff: { sortable: 1, name: 'selfDiff', label: 'Self (Diff)' },
  totalLeft: { sortable: 1, name: 'totalLeft', label: 'Total (Left)' },
  totalRght: { sortable: 1, name: 'totalRght', label: 'Total (Right)' },
  totalDiff: { sortable: 1, name: 'totalDiff', label: 'Total (Diff)' },
};

const tableFormatDiff = ((def) => ({
  diff: [def.name, def.selfDiff, def.totalDiff],
  self: [def.name, def.selfLeft, def.selfRght],
  total: [def.name, def.totalLeft, def.totalRght],
}))(tableFormatDiffDef);

function Table({
  tableBodyRef,
  flamebearer,
  viewDiff,
  fitMode,
  handleTableItemClick,
  highlightQuery,
  selectedItem,
  palette,
}: ProfilerTableProps) {
  const tableFormat = !viewDiff ? tableFormatSingle : tableFormatDiff[viewDiff];
  const tableSortProps = useTableSort(tableFormat);
  const table = {
    headRow: tableFormat,
    ...getTableBody({
      flamebearer,
      sortBy: tableSortProps.sortBy,
      sortByDirection: tableSortProps.sortByDirection,
      viewDiff,
      fitMode,
      handleTableItemClick,
      highlightQuery,
      palette,
      selectedItem,
    }),
  };

  return (
    <TableUI
      /* eslint-disable-next-line react/jsx-props-no-spreading */
      {...tableSortProps}
      tableBodyRef={tableBodyRef}
      table={table}
      className="flamegraph-table"
    />
  );
}

interface GetTableBodyRowsProps
  extends Omit<ProfilerTableProps, 'tableBodyRef'> {
  sortBy: string;
  sortByDirection: string;
}

const getTableBody = ({
  flamebearer,
  sortBy,
  sortByDirection,
  viewDiff,
  fitMode,
  handleTableItemClick,
  highlightQuery,
  palette,
  selectedItem,
}: GetTableBodyRowsProps): TableBodyType => {
  const { numTicks, maxSelf, sampleRate, spyName, units } = flamebearer;

  const tableBodyCells = generateTable(flamebearer).sort(
    (a, b) => b.total - a.total
  );
  const m = sortByDirection === 'asc' ? 1 : -1;
  let sorted: typeof tableBodyCells;

  if (sortBy === 'name') {
    sorted = tableBodyCells.sort(
      (a, b) => m * a[sortBy].localeCompare(b[sortBy])
    );
  } else {
    switch (sortBy) {
      case 'total':
      case 'self': {
        sorted = tableBodyCells.sort((a, b) => m * (a[sortBy] - b[sortBy]));
        break;
      }

      // sorting by all other fields means it must be a double
      default: {
        sorted = (tableBodyCells as (DoubleCell & { name: string })[]).sort(
          (a, b) =>
            m * (a[sortBy as DoubleSortByKeys] - b[sortBy as DoubleSortByKeys])
        );
      }
    }
  }

  const formatter = getFormatter(numTicks, sampleRate, units);
  const isRowSelected = (name: string) => {
    if (selectedItem.isJust) {
      return name === selectedItem.value;
    }

    return false;
  };

  const nameCell = (x: { name: string }, style: CSSProperties) => (
    <button className="table-item-button">
      <span className="color-reference" style={style} />
      <div className="symbol-name" style={fitIntoTableCell(fitMode)}>
        {x.name}
      </div>
    </button>
  );

  const getSingleRow = (
    x: SingleCell & { name: string },
    color: Color,
    style: CSSProperties
  ): BodyRow => ({
    'data-row': `${x.name};${x.self};${x.total};${x.type}`,
    isRowSelected: isRowSelected(x.name),
    onClick: () => handleTableItemClick(x),
    cells: [
      { value: nameCell(x, style) },
      {
        value: formatter.format(x.self, sampleRate),
        style: backgroundImageStyle(x.self, maxSelf, color),
      },
      {
        value: formatter.format(x.total, sampleRate),
        style: backgroundImageStyle(x.total, numTicks, color),
      },
    ],
  });

  const getDoubleRow = (() => {
    switch (viewDiff) {
      case 'self':
        return (
          x: DoubleCell & { name: string },
          color: Color,
          style: CSSProperties
        ): BodyRow & { type: 'self' } => ({
          type: 'self',
          isRowSelected: isRowSelected(x.name),
          onClick: () => handleTableItemClick(x),
          cells: [
            { value: nameCell(x, style) },
            {
              style: backgroundImageDiffStyle(
                palette,
                x.selfLeft,
                x.selfRght,
                maxSelf,
                color,
                'L'
              ),
              value: (() => (
                <span title={formatter.format(x.selfLeft, sampleRate)}>
                  {formatter.format(x.selfLeft, sampleRate)}
                </span>
              ))(),
            },
            {
              style: backgroundImageDiffStyle(
                palette,
                x.selfLeft,
                x.selfRght,
                maxSelf,
                color,
                'R'
              ),
              value: (
                <span title={formatter.format(x.selfRght, sampleRate)}>
                  {formatter.format(x.selfRght, sampleRate)}
                </span>
              ),
            },
          ],
        });
      case 'total':
        return (
          x: DoubleCell & { name: string },
          color: Color,
          style: CSSProperties
        ): BodyRow & { type: 'total' } => ({
          type: 'total',
          isRowSelected: isRowSelected(x.name),
          onClick: () => handleTableItemClick(x),
          cells: [
            { value: nameCell(x, style) },
            {
              style: backgroundImageDiffStyle(
                palette,
                x.totalLeft,
                x.totalRght,
                numTicks / 2,
                color,
                'L'
              ),
              value: (() => (
                <span title={formatter.format(x.totalLeft, sampleRate)}>
                  {formatter.format(x.totalLeft, sampleRate)}
                </span>
              ))(),
            },
            {
              style: backgroundImageDiffStyle(
                palette,
                x.totalLeft,
                x.totalRght,
                numTicks / 2,
                color,
                'R'
              ),
              value: (() => (
                <span title={formatter.format(x.totalRght, sampleRate)}>
                  {formatter.format(x.totalRght, sampleRate)}
                </span>
              ))(),
            },
          ],
        });
      case 'diff':
        return (
          x: DoubleCell & { name: string },
          color: Color,
          style: CSSProperties
        ): BodyRow & { type: 'diff' } => ({
          type: 'diff',
          isRowSelected: isRowSelected(x.name),
          onClick: () => handleTableItemClick(x),
          cells: [
            { value: nameCell(x, style) },
            {
              style: backgroundImageDiffStyle(
                palette,
                x.selfLeft,
                x.selfRght,
                maxSelf,
                defaultColor
              ),
              value: (() => (
                <span title={formatter.format(x.selfDiff, sampleRate)}>
                  {formatter.format(x.selfDiff, sampleRate)}
                </span>
              ))(),
            },
            {
              style: backgroundImageDiffStyle(
                palette,
                x.totalLeft,
                x.totalRght,
                numTicks / 2,
                color
              ),
              value: (() => (
                <span title={formatter.format(x.totalDiff, sampleRate)}>
                  {formatter.format(x.totalDiff, sampleRate)}
                </span>
              ))(),
            },
          ],
        });
      default:
        return (): { type: 'unsupported' } => ({
          type: 'unsupported',
        });
    }
  })();

  const rows = sorted
    .filter((x) => {
      if (!highlightQuery) {
        return true;
      }

      return isMatch(highlightQuery, x.name);
    })
    .reduce((acc, x) => {
      const pn = getPackageNameFromStackTrace(spyName, x.name);
      const color = viewDiff
        ? defaultColor
        : colorBasedOnPackageName(palette, pn);
      const style = {
        backgroundColor: color.rgb().toString(),
      };

      if (x.type === 'double') {
        const doubleRow = getDoubleRow(x, color, style);

        if (doubleRow.type === 'unsupported') return acc;

        acc.push(doubleRow);
        return acc;
      }

      acc.push(getSingleRow(x, color, style));
      return acc;
    }, [] as BodyRow[]);

  return rows.length > 0
    ? { bodyRows: rows, type: 'filled' as const }
    : {
        value: <div className="unsupported-format">Unsupported</div>,
        type: 'not-filled' as const,
      };
};

export interface ProfilerTableProps {
  flamebearer: Flamebearer;
  viewDiff?: 'diff' | 'total' | 'self' | false;
  fitMode: FitModes;
  handleTableItemClick: (tableItem: { name: string }) => void;
  highlightQuery: string;
  palette: FlamegraphPalette;
  selectedItem: Maybe<string>;

  tableBodyRef: RefObject<HTMLTableSectionElement>;
}

export default function ProfilerTable({
  flamebearer,
  viewDiff,
  fitMode,
  handleTableItemClick,
  highlightQuery,
  palette,
  selectedItem,
}: Omit<ProfilerTableProps, 'tableBodyRef'>) {
  const tableBodyRef = useRef<HTMLTableSectionElement>(null);

  return (
    <>
      <Table
        tableBodyRef={tableBodyRef}
        flamebearer={flamebearer}
        viewDiff={viewDiff}
        fitMode={fitMode}
        highlightQuery={highlightQuery}
        handleTableItemClick={handleTableItemClick}
        palette={palette}
        selectedItem={selectedItem}
      />
      <TableTooltip
        tableBodyRef={tableBodyRef}
        numTicks={flamebearer.numTicks}
        sampleRate={flamebearer.sampleRate}
        units={flamebearer.units}
      />
    </>
  );
}
