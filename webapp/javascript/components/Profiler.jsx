import React, { useState, useEffect, useRef } from "react";
import { useDispatch, useSelector } from "react-redux";
import { withShortcut } from "react-keybind";
import clsx from "clsx";
import ProfilerFlameGraph from "./ProfilerFlameGraph";
import ProfilerHeader from "./ProfilerHeader";
import ProfilerTable from "./ProfilerTable";
import { fetchJSON } from "../redux/actions";
import { buildRenderURL } from "../util/update_requests";
import { colorBasedOnPackageName, colorGreyscale } from "../util/color";

const Profiler = withShortcut(({ shortcut }) => {
  const dispatch = useDispatch();
  const tooltipRef = useRef(null);
  const canvasRef = useRef(null);
  const flamebearer = useSelector((state) => state.flamebearer);
  const [viewState, setViewState] = useState({
    highlightStyle: { display: "none" },
    tooltipStyle: { display: "none" },
    resetStyle: { visibility: "hidden" },
    sortBy: "self",
    sortByDirection: "desc",
    view: "both",
  });
  const [canvas, setCanvas] = useState(canvasRef.current);
  const [ctx, setCTX] = useState(canvas && canvas.getContext("2d"));
  const [canvasData, setCanvasData] = useState({
    topLevel: 0,
    selectedLevel: 0,
    rangeMin: 0,
    rangeMax: 1,
    query: "",
  });
  const from = useSelector((state) => state.from);
  const until = useSelector((state) => state.until);
  const labels = useSelector((state) => state.labels);

  useEffect(() => {
    dispatch(fetchJSON(buildRenderURL({ from, until, labels })));
  }, [from, until, labels]);

  useEffect(() => {
    updateData(flamebearer);
  }, [flamebearer]);

  useEffect(() => {
    if (shortcut) {
      shortcut.registerShortcut(
        reset,
        ["escape"],
        "Reset",
        "Reset Flamegraph View"
      );
    }
  }, []);

  const resizeHandler = () => {};
  const focusHandler = () => {};

  useEffect(() => {
    window.addEventListener("resize", resizeHandler);
    window.addEventListener("focus", focusHandler);
  }, [resizeHandler, focusHandler]);

  return (
    <div className="canvas-renderer">
      <div className="canvas-container">
        <ProfilerHeader
          view={viewState.view}
          handleSearchChange={() => {}}
          reset={() => {}}
          updateView={() => {}}
          resetStyle={viewState.resetStyle}
        />
        <div className="flamegraph-container panes-wrapper">
          <ProfilerTable
            flamebearer={flamebearer}
            viewState={viewState}
            sortByDirection={viewState.sortByDirection}
            sortBy={viewState.sortBy}
            setState={setViewState}
          />
          {/* <ProfilerFlameGraph
            view={view}
            canvasRef={canvasRef}
            clickHandler={clickHandler}
            mouseMoveHandler={mouseMoveHandler}
            mouseOutHandler={mouseOutHandler}
          /> */}
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
      <div className="flamegraph-highlight" style={viewState.highlightStyle} />
      <div
        className="flamegraph-tooltip"
        ref={tooltipRef}
        style={viewState.tooltipStyle}
      >
        <div className="flamegraph-tooltip-name">{viewState.tooltipTitle}</div>
        <div>{viewState.tooltipSubtitle}</div>
      </div>
    </div>
  );

  function updateData(flamebearer) {
    if (flamebearer) {
      // const { names, levels, numTicks, sampleRate } = flamebearer;
      setCanvasData(flamebearer);
      renderCanvas(flamebearer, canvasData);
    }
  }

  function rect(ctx, x, y, w, h, radius) {
    return ctx.rect(x, y, w, h);
  }

  function roundRect(ctx, x, y, w, h, radius) {
    if (radius >= w / 2) {
      return rect(ctx, x, y, w, h, radius);
    }
    radius = Math.min(w / 2, radius);
    const r = x + w;
    const b = y + h;
    ctx.beginPath();
    ctx.moveTo(x + radius, y);
    ctx.lineTo(r - radius, y);
    ctx.quadraticCurveTo(r, y, r, y + radius);
    ctx.lineTo(r, y + h - radius);
    ctx.quadraticCurveTo(r, b, r - radius, b);
    ctx.lineTo(x + radius, b);
    ctx.quadraticCurveTo(x, b, x, b - radius);
    ctx.lineTo(x, y + radius);
    ctx.quadraticCurveTo(x, y, x + radius, y);
  }

  function updateResetStyle({ selectedLevel, setViewState }) {
    // const emptyQuery = this.query === "";
    setViewState({
      resetStyle: { visibility: selectedLevel === 0 ? "hidden" : "visible" },
    });
  }

  function updateZoom(i, j) {
    if (!Number.isNaN(i) && !Number.isNaN(j)) {
      this.selectedLevel = i;
      this.topLevel = 0;
      this.rangeMin = this.levels[i][j] / this.numTicks;
      this.rangeMax =
        (this.levels[i][j] + this.levels[i][j + 1]) / this.numTicks;
    } else {
      this.selectedLevel = 0;
      this.topLevel = 0;
      this.rangeMin = 0;
      this.rangeMax = 1;
    }
    updateResetStyle({ selectedLevel, setViewState });
  }

  function renderCanvas(
    { names, levels, numTicks, sampleRate, spyName },
    { topLevel, selectedLevel, rangeMin, rangeMax, query }
  ) {
    if (!names || !canvas) {
      return;
    }

    const graphWidth = (canvas.width = canvas.clientWidth);
    const pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);
    canvas.height = PX_PER_LEVEL * (levels.length - topLevel);
    canvas.style.height = `${canvas.height}px`;
    const tickToX = (i) => (i - numTicks * rangeMin) * pxPerTick;

    if (devicePixelRatio > 1) {
      canvas.width *= 2;
      canvas.height *= 2;
      ctx.scale(2, 2);
    }

    ctx.textBaseline = "middle";
    ctx.font =
      '400 12px system-ui, -apple-system, "Segoe UI", "Roboto", "Ubuntu", "Cantarell", "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"';

    const df = new DurationFormater(numTicks / sampleRate);
    // i = level
    for (let i = 0; i < levels.length - topLevel; i++) {
      const level = levels[topLevel + i];

      for (let j = 0; j < level.length; j += 4) {
        // j = 0: x start of bar
        // j = 1: width of bar
        // j = 2: position in the main index

        const barIndex = level[j];
        const x = tickToX(barIndex);
        const y = i * PX_PER_LEVEL;
        let numBarTicks = level[j + 1];

        // For this particular bar, there is a match
        const queryExists = query.length > 0;
        const nodeIsInQuery =
          (query && names[level[j + 3]].indexOf(query) >= 0) || false;
        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * pxPerTick <= COLLAPSE_THRESHOLD;

        // const collapsed = false;
        if (collapsed) {
          while (
            j < level.length - 3 &&
            barIndex + numBarTicks === level[j + 3] &&
            level[j + 4] * pxPerTick <= COLLAPSE_THRESHOLD &&
            nodeIsInQuery ===
              ((query && names[level[j + 5]].indexOf(query) >= 0) || false)
          ) {
            j += 4;
            numBarTicks += level[j + 1];
          }
        }
        // ticks are samples
        const sw = numBarTicks * pxPerTick - (collapsed ? 0 : GAP);
        const sh = PX_PER_LEVEL - GAP;

        // if (x < -1 || x + sw > this.graphWidth + 1 || sw < HIDE_THRESHOLD) continue;

        ctx.beginPath();
        rect(ctx, x, y, sw, sh, 3);

        const ratio = numBarTicks / numTicks;

        const a = selectedLevel > i ? 0.33 : 1;

        let nodeColor;
        if (collapsed) {
          nodeColor = colorGreyscale(200, 0.66);
        } else if (queryExists && nodeIsInQuery) {
          nodeColor = HIGHLIGHT_NODE_COLOR;
        } else if (queryExists && !nodeIsInQuery) {
          nodeColor = colorGreyscale(200, 0.66);
        } else {
          nodeColor = colorBasedOnPackageName(
            getPackageNameFromStackTrace(spyName, names[level[j + 3]]),
            a
          );
        }

        ctx.fillStyle = nodeColor;
        ctx.fill();

        if (!collapsed && sw >= LABEL_THRESHOLD) {
          const percent = formatPercent(ratio);
          const name = `${names[level[j + 3]]} (${percent}, ${df.format(
            numBarTicks / sampleRate
          )})`;

          ctx.save();
          ctx.clip();
          ctx.fillStyle = "black";
          ctx.fillText(name, Math.round(Math.max(x, 0) + 3), y + sh / 2);
          ctx.restore();
        }
      }
    }
  }

  function getPackageNameFromStackTrace(spyName, stackTrace) {
    const regexpLookup = {
      pyspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/,
      rbspy: /^(?<func>.+? - )?(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
      gospy: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
      default: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
    };

    if (stackTrace.length == 0) {
      return stackTrace;
    }
    const regexp = regexpLookup[spyName] || regexpLookup.default;
    const fullStackGroups = stackTrace.match(regexp);
    if (fullStackGroups) {
      return fullStackGroups.groups.packageName;
    }
    return stackTrace;
  }
});

export default withShortcut(Profiler);
