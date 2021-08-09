/* eslint no-nested-ternary: 0 */
/* eslint prettier/prettier: 0 */

import React from "react";
import clsx from "clsx";
import { getFormatter, getPackageNameFromStackTrace } from "../util/format";
import { colorBasedOnPackageName, defaultColor, diffColorGreen, diffColorRed } from "../util/color";
import { parseFlamebearerFormat } from "../util/flamebearer";

const zero = (v) => v || 0;

function generateCellSingle(ff, cell, level, j) {
  const c = cell;
  c.self = zero(c.self) + ff.getBarSelf(level, j);
  c.total = zero(c.total) + ff.getBarTotal(level, j);
  return c;
}

function generateCellDouble(ff, cell, level, j) {
  const c = cell;
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
const generateTable = (flamebearer) => {
  const table = [];
  if (!flamebearer) {
    return table;
  }
  const { names, levels, format } = flamebearer;
  const ff = parseFlamebearerFormat(format);
  const generateCell =
    format !== "double" ? generateCellSingle : generateCellDouble;

  const hash = {};
  // eslint-disable-next-line no-plusplus
  for (let i = 0; i < levels.length; i++) {
    const level = levels[i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const key = ff.getBarName(level, j);
      const name = names[key];
      hash[name] = hash[name] || { name: name || "<empty>" };
      generateCell(ff, hash[name], level, j);
    }
  }
  return Object.values(hash);
};

// the value must be negative or zero
function neg(v) { return Math.min(0, v); }

function backgroundImageStyle(a, b, color) {
  const w = 148;
  const k = w - (a / b) * w;
  const clr = color.alpha(1.0);
  return {
    // backgroundColor: 'transparent',
    backgroundImage: `linear-gradient(${clr}, ${clr})`,
    backgroundPosition: `-${k}px 0px`,
    backgroundRepeat: "no-repeat",
  };
}

// side: _ | L | R : indicates how to render the diff color
// - _: render both diff color
// - L: only render diff color on the left, if the left is longer than the right (better, green)
// - R: only render diff color on the right, if the right is longer than the left (worse, red)
function backgroundImageDiffStyle(a, b, total, color, side) {
  const w = 148;
  const k = w - (Math.min(a, b) / total) * w;
  const kd = w - (Math.max(a, b) / total) * w;
  const clr = color.alpha(1.0);
  const cld = b < a ? diffColorGreen.alpha(0.8)
                    : diffColorRed.alpha(0.8);

  if (side === 'L' && a < b) {
    return `
    background-image: linear-gradient(${clr}, ${clr});
    background-position: ${neg(-k)}px 0px;
    background-repeat: no-repeat;
  `;
  }
  if (side === 'R' && b < a) {
    return `
    background-image: linear-gradient(${clr}, ${clr});
    background-position: ${neg(-k)}px 0px;
    background-repeat: no-repeat;
  `;
  }

  // NOTE: it seems React does not understand multiple backgrounds, have to workaround
  return `
    background-image: linear-gradient(${clr}, ${clr}), linear-gradient(${cld}, ${cld});
    background-position: ${neg(-k)}px 0px, ${neg(-kd)}px 0px;
    background-repeat: no-repeat;
  `;
}

export default function ProfilerTable({
  flamebearer,
  sortByDirection,
  sortBy,
  updateSortBy,
  viewDiff,
}) {
  return (
    <Table
      flamebearer={flamebearer}
      updateSortBy={updateSortBy}
      sortBy={sortBy}
      sortByDirection={sortByDirection}
      viewDiff={viewDiff}
    />
  );
}

const tableFormatSingle = [
  { sortable: 1, name: "name", label: "Location" },
  { sortable: 1, name: "self", label: "Self" },
  { sortable: 1, name: "total", label: "Total" },
];

const tableFormatDiffDef = {
  name:      { sortable: 1, name: "name", label: "Location" },
  selfLeft:  { sortable: 1, name: "selfLeft", label: "Self (Left)" },
  selfRght:  { sortable: 1, name: "selfRght", label: "Self (Right)" },
  selfDiff:  { sortable: 1, name: "selfDiff", label: "Self (Diff)" },
  totalLeft: { sortable: 1, name: "totalLeft", label: "Total (Left)" },
  totalRght: { sortable: 1, name: "totalRght", label: "Total (Right)" },
  totalDiff: { sortable: 1, name: "totalDiff", label: "Total (Diff)" },
};

const tableFormatDiff = ((def) => ({
  diff:  [def.name, def.selfDiff,  def.totalDiff],
  self:  [def.name, def.selfLeft,  def.selfRght],
  total: [def.name, def.totalLeft, def.totalRght],
}))(tableFormatDiffDef);

function Table({ flamebearer, updateSortBy, sortBy, sortByDirection, viewDiff }) {
  if (!flamebearer || flamebearer.numTicks === 0) {
    return [];
  }
  const tableFormat =
    !viewDiff ? tableFormatSingle : tableFormatDiff[viewDiff];

  return (
    <table className="flamegraph-table">
      <thead>
        <tr>
          {tableFormat.map((v, idx) =>
            !v.sortable ? (
              // eslint-disable-next-line react/no-array-index-key
              <th key={idx}>{v.label}</th>
            ) : (
              <th
                // eslint-disable-next-line react/no-array-index-key
                key={idx}
                className="sortable"
                onClick={() => updateSortBy(v.name)}
              >
                {v.label}
                <span
                  className={clsx("sort-arrow", {
                    [sortByDirection]: sortBy === v.name,
                  })}
                />
              </th>
            )
          )}
        </tr>
      </thead>
      <tbody>
        <TableBody
          flamebearer={flamebearer}
          sortBy={sortBy}
          sortByDirection={sortByDirection}
          viewDiff={viewDiff}
        />
      </tbody>
    </table>
  );
}

function TableBody({ flamebearer, sortBy, sortByDirection, viewDiff }) {
  const { numTicks, maxSelf, sampleRate, spyName, units } = flamebearer;

  const table = generateTable(flamebearer).sort((a, b) => b.total - a.total);

  const m = sortByDirection === "asc" ? 1 : -1;
  let sorted;
  if (sortBy === "name") {
    sorted = table.sort((a, b) => m * a[sortBy].localeCompare(b[sortBy]));
  } else {
    sorted = table.sort((a, b) => m * (a[sortBy] - b[sortBy]));
  }

  // The problem is that when you switch apps or time-range and the function
  //   names stay the same it leads to an issue where rows don't get re-rendered
  // So we force a rerender each time.
  const renderID = Math.random();

  const formatter = getFormatter(numTicks, sampleRate, units);

  const renderRow =
    !viewDiff ? (x, color, style) => (
      <tr key={x.name + renderID}>
        <td>
          <span className="color-reference" style={style} />
          <span title={x.name}>{x.name}</span>
        </td>
        <td style={backgroundImageStyle(x.self, maxSelf, color)}>
          {/* <span>{ formatPercent(x.self / numTicks) }</span>
      &nbsp;
      <span>{ shortNumber(x.self) }</span>
      &nbsp; */}
          <span title={formatter.format(x.self, sampleRate)}>
            {formatter.format(x.self, sampleRate)}
          </span>
        </td>
        <td style={backgroundImageStyle(x.total, numTicks, color)}>
          {/* <span>{ formatPercent(x.total / numTicks) }</span>
      &nbsp;
      <span>{ shortNumber(x.total) }</span>
      &nbsp; */}
          <span title={formatter.format(x.total, sampleRate)}>
            {formatter.format(x.total, sampleRate)}
          </span>
        </td>
      </tr>
    ) : viewDiff === 'self' ? (x, color, style) => (
      <tr key={x.name + renderID}>
        <td>
          <span className="color-reference" style={style} />
          <span title={x.name}>{x.name}</span>
        </td>
        {/* NOTE: it seems React does not understand multiple backgrounds, have to workaround:  */}
        {/*   The `style` prop expects a mapping from style properties to values, not a string. */}
        <td STYLE={backgroundImageDiffStyle(x.selfLeft, x.selfRght, maxSelf, color, 'L')}>
          <span title={formatter.format(x.selfLeft, sampleRate)}>
            {formatter.format(x.selfLeft, sampleRate)}
          </span>
        </td>
        <td STYLE={backgroundImageDiffStyle(x.selfLeft, x.selfRght, maxSelf, color, 'R')}>
          <span title={formatter.format(x.selfRght, sampleRate)}>
            {formatter.format(x.selfRght, sampleRate)}
          </span>
        </td>
      </tr>
    ) : viewDiff === 'total' ? (x, color, style) => (
      <tr key={x.name + renderID}>
        <td>
          <span className="color-reference" style={style} />
          <span title={x.name}>{x.name}</span>
        </td>
        <td STYLE={backgroundImageDiffStyle(x.totalLeft, x.totalRght, numTicks / 2, color, 'L')}>
          <span title={formatter.format(x.totalLeft, sampleRate)}>
            {formatter.format(x.totalLeft, sampleRate)}
          </span>
        </td>
        <td STYLE={backgroundImageDiffStyle(x.totalLeft, x.totalRght, numTicks / 2, color, 'R')}>
          <span title={formatter.format(x.totalRght, sampleRate)}>
            {formatter.format(x.totalRght, sampleRate)}
          </span>
        </td>
      </tr>
    ) : viewDiff === 'diff' ? (x, color, style) => (
      <tr key={x.name + renderID}>
        <td>
          <span className="color-reference" style={style} />
          <span title={x.name}>{x.name}</span>
        </td>
        <td STYLE={backgroundImageDiffStyle(x.selfLeft, x.selfRght, maxSelf, defaultColor)}>
          <span title={formatter.format(x.selfDiff, sampleRate)}>
            {formatter.format(x.selfDiff, sampleRate)}
          </span>
        </td>
        <td STYLE={backgroundImageDiffStyle(x.totalLeft, x.totalRght, numTicks / 2, color)}>
          <span title={formatter.format(x.totalDiff, sampleRate)}>
            {formatter.format(x.totalDiff, sampleRate)}
          </span>
        </td>
      </tr>
    ) : <div>invalid</div>;

  return sorted.map((x) => {
    const pn = getPackageNameFromStackTrace(spyName, x.name);
    const color = viewDiff ? defaultColor : colorBasedOnPackageName(pn, 1);
    const style = {
      backgroundColor: color,
    };
    return renderRow(x, color, style);
  });
}
