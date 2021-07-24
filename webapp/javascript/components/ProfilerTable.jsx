import React from "react";
import clsx from "clsx";
import { getFormatter, getPackageNameFromStackTrace } from "../util/format";
import { colorBasedOnPackageName } from "../util/color";
import { parseFlamebearerFormat } from "../util/flamebearer";

// generates a table from data in flamebearer format
const generateTable = (flamebearer) => {
  const table = [];
  if (!flamebearer) {
    return table;
  }
  const { names, levels, format } = flamebearer;
  const ff = parseFlamebearerFormat(format);
  const hash = {};
  // eslint-disable-next-line no-plusplus
  for (let i = 0; i < levels.length; i++) {
    const level = levels[i];
    for (let j = 0; j < level.length; j += ff.jStep) {
      const key = ff.getBarName(level, j);
      const name = names[key];
      hash[name] = hash[name] || {
        name: name || "<empty>",
        self: 0,
        total: 0,
      };
      hash[name].total += ff.getBarTotal(level, j);
      hash[name].self += ff.getBarSelf(level, j);
    }
  }
  return Object.values(hash);
};

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

export default function ProfilerTable({
  flamebearer,
  sortByDirection,
  sortBy,
  updateSortBy,
}) {
  return (
    <Table
      flamebearer={flamebearer}
      updateSortBy={updateSortBy}
      sortBy={sortBy}
      sortByDirection={sortByDirection}
    />
  );
}

function Table({ flamebearer, updateSortBy, sortBy, sortByDirection }) {
  if (!flamebearer || flamebearer.numTicks === 0) {
    return [];
  }

  return (
    <table className="flamegraph-table">
      <thead>
        <tr>
          <th className="sortable" onClick={() => updateSortBy("name")}>
            Location
            <span
              className={clsx("sort-arrow", {
                [sortByDirection]: sortBy === "name",
              })}
            />
          </th>
          <th className="sortable" onClick={() => updateSortBy("self")}>
            Self
            <span
              className={clsx("sort-arrow", {
                [sortByDirection]: sortBy === "self",
              })}
            />
          </th>
          <th className="sortable" onClick={() => updateSortBy("total")}>
            Total
            <span
              className={clsx("sort-arrow", {
                [sortByDirection]: sortBy === "total",
              })}
            />
          </th>
        </tr>
      </thead>
      <tbody>
        <TableBody
          flamebearer={flamebearer}
          sortBy={sortBy}
          sortByDirection={sortByDirection}
        />
      </tbody>
    </table>
  );
}

function TableBody({ flamebearer, sortBy, sortByDirection }) {
  const { numTicks, maxSelf, sampleRate, spyName, units, format } = flamebearer;

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
  let renderID = Math.random();

  const formatter = getFormatter(numTicks, sampleRate, units);

  return sorted.map((x) => {
    const pn = getPackageNameFromStackTrace(spyName, x.name);
    const color = colorBasedOnPackageName(pn, 1);
    const style = {
      backgroundColor: color,
    };
    return (
      <tr key={x.name+renderID}>
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
    );
  });
}
