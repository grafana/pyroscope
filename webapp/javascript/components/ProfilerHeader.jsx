import React from "react";
import clsx from "clsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faIcicles,
  faColumns,
  faBars,
} from "@fortawesome/free-solid-svg-icons";

export default function ProfilerHeader({
  view,
  handleSearchChange,
  resetStyle,
  reset,
  updateView,
}) {
  return (
    <div className="navbar-2">
      <input
        className="flamegraph-search"
        name="flamegraph-search"
        placeholder="Search…"
        onChange={handleSearchChange}
      />
      &nbsp;
      <button
        className={clsx("btn")}
        style={resetStyle}
        id="reset"
        onClick={reset}
      >
        Reset View
      </button>
      <div className="navbar-space-filler" />
      <div className="btn-group viz-switch">
        <button
          className={clsx("btn", { active: view === "table" })}
          onClick={() => updateView("table")}
        >
          <FontAwesomeIcon icon={faBars} />
          &nbsp;&thinsp;Table
        </button>
        <button
          className={clsx("btn", { active: view === "both" })}
          onClick={() => updateView("both")}
        >
          <FontAwesomeIcon icon={faColumns} />
          &nbsp;&thinsp;Both
        </button>
        <button
          className={clsx("btn", { active: view === "icicle" })}
          onClick={() => updateView("icicle")}
        >
          <FontAwesomeIcon icon={faIcicles} />
          &nbsp;&thinsp;Flamegraph
        </button>
      </div>
    </div>
  );
}
