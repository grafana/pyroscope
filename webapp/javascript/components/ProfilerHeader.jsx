import React from "react";
import clsx from "clsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faAlignLeft,
  faBars,
  faColumns,
  faIcicles,
  faListUl,
  faTable,
} from "@fortawesome/free-solid-svg-icons";

export default function ProfilerHeader({
  view,
  viewDiff,
  handleSearchChange,
  resetStyle,
  reset,
  updateView,
  updateViewDiff,
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
        type="button"
        className={clsx("btn")}
        style={resetStyle}
        id="reset"
        onClick={reset}
      >
        Reset View
      </button>
      <div className="navbar-space-filler" />
      {!viewDiff ? null : (
        <div className="btn-group viz-switch">
          <button
            type="button"
            className={clsx("btn", { active: viewDiff === "self" })}
            onClick={() => updateViewDiff("self")}
          >
            <FontAwesomeIcon icon={faListUl} />
            &nbsp;&thinsp;Self
          </button>
          <button
            type="button"
            className={clsx("btn", { active: viewDiff === "total" })}
            onClick={() => updateViewDiff("total")}
          >
            <FontAwesomeIcon icon={faBars} />
            &nbsp;&thinsp;Total
          </button>
          <button
            type="button"
            className={clsx("btn", { active: viewDiff === "diff" })}
            onClick={() => updateViewDiff("diff")}
          >
            <FontAwesomeIcon icon={faAlignLeft} />
            &nbsp;&thinsp;Diff
          </button>
        </div>
      )}
      <div className="btn-group viz-switch">
        <button
          type="button"
          className={clsx("btn", { active: view === "table" })}
          onClick={() => updateView("table")}
        >
          <FontAwesomeIcon icon={faTable} />
          &nbsp;&thinsp;Table
        </button>
        <button
          type="button"
          className={clsx("btn", { active: view === "both" })}
          onClick={() => updateView("both")}
        >
          <FontAwesomeIcon icon={faColumns} />
          &nbsp;&thinsp;Both
        </button>
        <button
          type="button"
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
