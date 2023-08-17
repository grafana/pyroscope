import React, { useRef, RefObject, CSSProperties } from 'react';
import type Color from 'color';
import cl from 'classnames';
import type { Maybe } from 'true-myth';
import { doubleFF, singleFF, Flamebearer } from '@pyroscope/legacy/models';
// until ui is moved to its own package this should do it
// eslint-disable-next-line import/no-extraneous-dependencies
import TableUI, {
  useTableSort,
  BodyRow,
  TableBodyType,
} from '@pyroscope/ui/Table';
import TableTooltip from './Tooltip/TableTooltip';
import { getFormatter, ratioToPercent, diffPercent } from './format/format';
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
  leftTicks: number;
  rightTicks: number;
}
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
  j: number,
  leftTicks: number,
  rightTicks: number
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
  c.leftTicks = leftTicks;
  c.rightTicks = rightTicks;
  return c;
}

// generates a table from data in flamebearer format
function generateTable(
  flamebearer: Flamebearer
): Array<(SingleCell | DoubleCell) & { name: string }> {
  const table: Array<(SingleCell | DoubleCell) & { name: string }> = [];
  if (!flamebearer) {
    return table;
  }
  const { names, levels, format } = flamebearer;
  const ff = format !== 'double' ? singleFF : doubleFF;

  const hash = new Map<string, (DoubleCell | SingleCell) & { name: string }>();
  // eslint-disable-next-line no-plusplus
  for (let i = 0; i < levels.length; i++) {
    const level = levels[i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const key = ff.getBarName(level, j);
      const name = names[key];

      if (!hash.has(name)) {
        hash.set(name, {
          name: name || '<empty>',
          self: 0,
          total: 0,
        } as SingleCell & { name: string });
      }

      const cell = hash.get(name);
      // Should not happen
      if (!cell) {
        break;
      }

      // TODO(eh-am): not the most optimal performance wise
      // but better for type checking
      if (format === 'single') {
        generateCellSingle(singleFF, cell as SingleCell, level, j);
      } else {
        generateCellDouble(
          doubleFF,
          cell as DoubleCell,
          level,
          j,
          flamebearer.leftTicks,
          flamebearer.rightTicks
        );
      }
    }
  }

  return Array.from(hash.values());
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
export function backgroundImageDiffStyle(
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

const tableFormatSingle: Array<{
  sortable: number;
  name: 'name' | 'self' | 'total';
  label: string;
  default?: boolean;
}> = [
  { sortable: 1, name: 'name', label: 'Location' },
  { sortable: 1, name: 'self', label: 'Self', default: true },
  { sortable: 1, name: 'total', label: 'Total' },
];

const tableFormatDouble: Array<{
  sortable: number;
  name: 'name' | 'baseline' | 'comparison' | 'diff';
  label: string;
  default?: boolean;
}> = [
  { sortable: 1, name: 'name', label: 'Location' },
  { sortable: 1, name: 'baseline', label: 'Baseline', default: true },
  { sortable: 1, name: 'comparison', label: 'Comparison' },
  { sortable: 1, name: 'diff', label: 'Diff' },
];

function Table({
  tableBodyRef,
  flamebearer,
  isDoubles,
  fitMode,
  handleTableItemClick,
  highlightQuery,
  selectedItem,
  palette,
}: ProfilerTableProps & { isDoubles: boolean }) {
  const tableFormat = isDoubles ? tableFormatDouble : tableFormatSingle;
  const tableSortProps = useTableSort(tableFormat);
  const table = {
    headRow: tableFormat,
    ...getTableBody({
      flamebearer,
      sortBy: tableSortProps.sortBy,
      sortByDirection: tableSortProps.sortByDirection,
      isDoubles,
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
      className={cl('flamegraph-table', {
        'flamegraph-table-doubles': isDoubles,
      })}
    />
  );
}

interface GetTableBodyRowsProps
  extends Omit<ProfilerTableProps, 'tableBodyRef'> {
  sortBy: string;
  sortByDirection: string;
  isDoubles: boolean;
}

const getTableBody = ({
  flamebearer,
  sortBy,
  sortByDirection,
  isDoubles,
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
      case 'baseline': {
        sorted = (tableBodyCells as Array<DoubleCell & { name: string }>).sort(
          (a, b) => m * (a.totalLeft / a.leftTicks - b.totalLeft / b.leftTicks)
        );
        break;
      }
      case 'comparison': {
        sorted = (tableBodyCells as Array<DoubleCell & { name: string }>).sort(
          (a, b) =>
            m * (a.totalRght / a.rightTicks - b.totalRght / b.rightTicks)
        );
        break;
      }
      case 'diff': {
        sorted = (tableBodyCells as Array<DoubleCell & { name: string }>).sort(
          (a, b) => {
            const totalDiffA = diffPercent(
              ratioToPercent(a.totalLeft / a.leftTicks),
              ratioToPercent(a.totalRght / a.rightTicks)
            );
            const totalDiffB = diffPercent(
              ratioToPercent(b.totalLeft / b.leftTicks),
              ratioToPercent(b.totalRght / b.rightTicks)
            );

            return m * (totalDiffA - totalDiffB);
          }
        );
        break;
      }
      default:
        sorted = tableBodyCells;
        break;
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
    'data-row': `${x.type};${x.name};${x.self};${x.total}`,
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

  const getDoubleRow = (
    x: DoubleCell & { name: string },
    style: CSSProperties
  ): BodyRow => {
    const leftPercent = ratioToPercent(x.totalLeft / x.leftTicks);
    const rghtPercent = ratioToPercent(x.totalRght / x.rightTicks);

    const totalDiff = diffPercent(leftPercent, rghtPercent);

    let diffCellColor = '';
    if (totalDiff > 0) {
      diffCellColor = palette.badColor.rgb().string();
    } else if (totalDiff < 0) {
      diffCellColor = palette.goodColor.rgb().string();
    }

    let diffValue = '';
    if (!x.totalLeft || totalDiff === Infinity) {
      // this is a new function
      diffValue = '(new)';
    } else if (!x.totalRght) {
      // this function has been removed
      diffValue = '(removed)';
    } else if (totalDiff > 0) {
      diffValue = `(+${totalDiff.toFixed(2)}%)`;
    } else if (totalDiff < 0) {
      diffValue = `(${totalDiff.toFixed(2)}%)`;
    }

    return {
      'data-row': `${x.type};${x.name};${x.totalLeft};${x.leftTicks};${x.totalRght};${x.rightTicks}`,
      isRowSelected: isRowSelected(x.name),
      onClick: () => handleTableItemClick(x),
      cells: [
        { value: nameCell(x, style) },
        { value: `${leftPercent} %` },
        { value: `${rghtPercent} %` },
        {
          value: diffValue,
          style: {
            color: diffCellColor,
          },
        },
      ],
    };
  };

  const rows = sorted
    .filter((x) => {
      if (!highlightQuery) {
        return true;
      }

      return isMatch(highlightQuery, x.name);
    })
    .map((x) => {
      const pn = getPackageNameFromStackTrace(spyName, x.name);
      const color = isDoubles
        ? defaultColor
        : colorBasedOnPackageName(palette, pn);
      const style = {
        backgroundColor: color.rgb().toString(),
      };

      if (x.type === 'double') {
        return getDoubleRow(x, style);
      }

      return getSingleRow(x, color, style);
    });

  return rows.length > 0
    ? { bodyRows: rows, type: 'filled' as const }
    : {
        value: <div className="unsupported-format">No items found</div>,
        type: 'not-filled' as const,
      };
};

export interface ProfilerTableProps {
  flamebearer: Flamebearer;
  fitMode: FitModes;
  handleTableItemClick: (tableItem: { name: string }) => void;
  highlightQuery: string;
  palette: FlamegraphPalette;
  selectedItem: Maybe<string>;

  tableBodyRef: RefObject<HTMLTableSectionElement>;
}

const ProfilerTable = React.memo(function ProfilerTable({
  flamebearer,
  fitMode,
  handleTableItemClick,
  highlightQuery,
  palette,
  selectedItem,
}: Omit<ProfilerTableProps, 'tableBodyRef'>) {
  const tableBodyRef = useRef<HTMLTableSectionElement>(null);

  return (
    <div data-testid="table-view">
      <Table
        tableBodyRef={tableBodyRef}
        flamebearer={flamebearer}
        isDoubles={flamebearer.format === 'double'}
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
        palette={palette}
      />
    </div>
  );
});

export default ProfilerTable;
