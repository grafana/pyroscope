import React from "react";
import clsx from "clsx";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faAlignLeft } from "@fortawesome/free-solid-svg-icons/faAlignLeft";
import { faBars } from "@fortawesome/free-solid-svg-icons/faBars";
import { faColumns } from "@fortawesome/free-solid-svg-icons/faColumns";
import { faIcicles } from "@fortawesome/free-solid-svg-icons/faIcicles";
import { faListUl } from "@fortawesome/free-solid-svg-icons/faListUl";
import { faTable } from "@fortawesome/free-solid-svg-icons/faTable";
import { FitModes } from "../util/fitMode";
import { debounce } from "lodash";
import { useCallback, useState } from "react";

export default function ProfilerHeader({
  view,
  viewDiff,
  handleSearchChange,
  resetStyle,
  reset,
  updateFitMode,
  fitMode,
  updateView,
  updateViewDiff,
}) {

  // debounce the search
  // since rebuilding the canvas on each keystroke is expensive
//  const deb = useCallback(debounce(e => handleSearchChange(e), 250, { maxWait: 1000 }), []);
  const onChange = (e) => {
    const q = e.target.value;
 //   deb(q);
    handleSearchChange(q);
  }

  return (
    <div className="navbar-2">
      <input
        className="flamegraph-search"
        name="flamegraph-search"
        placeholder="Search…"
        onChange={onChange}
      />
      &nbsp;
      <select
        className="fit-mode-select"
        value={fitMode}
        onChange={(event) => updateFitMode(event.target.value)}
      >
        <option disabled>Prefer to fit</option>
        <option value={FitModes.HEAD}>Head First</option>
        <option value={FitModes.TAIL}>Tail First</option>
      </select>
      <button
        type="button"
        className={clsx("btn")}
        style={resetStyle}
        data-testid="reset-view"
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
          data-testid="btn-table-view"
          className={clsx("btn", { active: view === "table" })}
          onClick={() => updateView("table")}
        >
          <FontAwesomeIcon icon={faTable} />
          &nbsp;&thinsp;Table
        </button>
        <button
          data-testid="btn-both-view"
          type="button"
          className={clsx("btn", { active: view === "both" })}
          onClick={() => updateView("both")}
        >
          <FontAwesomeIcon icon={faColumns} />
          &nbsp;&thinsp;Both
        </button>
        <button
          data-testid="btn-flamegraph-view"
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
