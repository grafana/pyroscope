import React, { useState, useEffect, useRef } from "react";
import { useDispatch, useSelector } from "react-redux";
import { withShortcut } from "react-keybind";
import clsx from "clsx";
import ProfilerFlameGraph from "./ProfilerFlameGraph";
import ProfilerHeader from "./ProfilerHeader";
import ProfilerTable from "./ProfilerTable";
import { fetchJSON } from "../redux/actions";
import { buildRenderURL } from "../util/update_requests";

const Profiler = withShortcut(({ shortcut }) => {
  const dispatch = useDispatch();
  const tooltipRef = useRef();
  const flamebearer = useSelector((state) => state.flamebearer);
  const [view, setView] = useState("both");
  const [state, setState] = useState({
    highlightStyle: { display: "none" },
    tooltipStyle: { display: "none" },
    resetStyle: { visibility: "hidden" },
    sortBy: "self",
    sortByDirection: "desc",
    view: "both",
  });

  const from = useSelector((state) => state.from);
  const until = useSelector((state) => state.until);
  const labels = useSelector((state) => state.labels);

  useEffect(() => {
    dispatch(fetchJSON(buildRenderURL({ from, until, labels })));
  }, [from, until, labels]);

  return (
    <div className="canvas-renderer">
      <div className="canvas-container">
        <ProfilerHeader
          view={view}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateView={() => {}}
          resetStyle={state.resetStyle}
        />
        <div className="flamegraph-container panes-wrapper">
          {/* <ProfilerTable
            flamebearer={flamebearer}
            view={view}
            sortByDirection={sortByDirection}
            sortBy={sortBy}
            setState={setState}
          /> */}
          {/* <ProfilerFlameGraph /> */}
        </div>
        <div
          className={clsx("no-data-message", {
            visible: flamebearer && flamebearer.numTicks === 0,
          })}
        >
          <span>
            No profiling data available for this application / time range.
          </span>
        </div>
      </div>
      <div className="flamegraph-highlight" style={state.highlightStyle} />
      <div
        className="flamegraph-tooltip"
        ref={tooltipRef}
        style={state.tooltipStyle}
      >
        <div className="flamegraph-tooltip-name">{state.tooltipTitle}</div>
        <div>{state.tooltipSubtitle}</div>
      </div>
    </div>
  );
});

export default withShortcut(Profiler);
