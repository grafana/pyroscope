import React from "react";
import clsx from "clsx";
import { DurationFormater } from "../util/format";
import { colorBasedOnPackageName } from "../util/color";

export default function ProfilerTable({
  flamebearer,
  sortByDirection,
  sortBy,
  updateSortBy,
  view,
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
  const { numTicks, maxSelf, sampleRate, spyName } = flamebearer;

  const table = generateTable(flamebearer).sort((a, b) => b.total - a.total);

  const m = sortByDirection === "asc" ? 1 : -1;
  let sorted;
  if (sortBy === "name") {
    sorted = table.sort((a, b) => m * a[sortBy].localeCompare(b[sortBy]));
  } else {
    sorted = table.sort((a, b) => m * (a[sortBy] - b[sortBy]));
  }

  const df = new DurationFormater(numTicks / sampleRate);

  return sorted.map((x) => {
    const pn = getPackageNameFromStackTrace(spyName, x.name);
    const color = colorBasedOnPackageName(pn, 1);
    const style = {
      backgroundColor: color,
    };
    return (
      <tr key={x.name}>
        <td>
          <span className="color-reference" style={style} />
          <span>{x.name}</span>
        </td>
        <td style={backgroundImageStyle(x.self, maxSelf, color)}>
          {/* <span>{ formatPercent(x.self / numTicks) }</span>
        &nbsp;
        <span>{ shortNumber(x.self) }</span>
        &nbsp; */}
          <span title={df.format(x.self / sampleRate)}>
            {df.format(x.self / sampleRate)}
          </span>
        </td>
        <td style={backgroundImageStyle(x.total, numTicks, color)}>
          {/* <span>{ formatPercent(x.total / numTicks) }</span>
        &nbsp;
        <span>{ shortNumber(x.total) }</span>
        &nbsp; */}
          <span title={df.format(x.total / sampleRate)}>
            {df.format(x.total / sampleRate)}
          </span>
        </td>
      </tr>
    );
  });
}

function getPackageNameFromStackTrace(spyName, stackTrace) {
  // TODO: actually make sure these make sense and add tests
  const regexpLookup = {
    pyspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/,
    rbspy: /^(?<func>.+? - )?(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
    gospy: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
    default: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
  };

  if (stackTrace.length === 0) {
    return stackTrace;
  }
  const regexp = regexpLookup[spyName] || regexpLookup.default;
  const fullStackGroups = stackTrace.match(regexp);
  if (fullStackGroups) {
    return fullStackGroups.groups.packageName;
  }
  return stackTrace;
}

// generates a table from data in flamebearer format
const generateTable = (flamebearer) => {
  const table = [];
  if (!flamebearer) {
    return table;
  }
  const { names, levels } = flamebearer;
  const hash = {};
  // eslint-disable-next-line no-plusplus
  for (let i = 0; i < levels.length; i++) {
    for (let j = 0; j < levels[i].length; j += 4) {
      const key = levels[i][j + 3];
      const name = names[key];
      hash[name] = hash[name] || {
        name: name || "<empty>",
        self: 0,
        total: 0,
      };
      hash[name].total += levels[i][j + 1];
      hash[name].self += levels[i][j + 2];
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
