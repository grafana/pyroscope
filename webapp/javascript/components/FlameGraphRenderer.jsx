/* eslint-disable */
// ISC License

// Copyright (c) 2018, Mapbox

// Permission to use, copy, modify, and/or distribute this software for any purpose
// with or without fee is hereby granted, provided that the above copyright notice
// and this permission notice appear in all copies.

// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
// REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
// FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
// INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
// OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
// TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
// THIS SOFTWARE.

// This component is based on flamebearer project
//   https://github.com/mapbox/flamebearer

import React from "react";
import { connect } from "react-redux";

import clsx from "clsx";

import { bindActionCreators } from "redux";

import { withShortcut } from "react-keybind";

import { fetchJSON } from "../redux/actions";
import { buildRenderURL } from "../util/update_requests";
import {
  numberWithCommas,
  formatPercent,
  DurationFormater,
} from "../util/format";
import { colorBasedOnPackageName, colorGreyscale } from "../util/color";
import ProfilerTable from "./ProfilerTable";
import ProfilerHeader from "./ProfilerHeader";

const PX_PER_LEVEL = 18;
const COLLAPSE_THRESHOLD = 5;
const LABEL_THRESHOLD = 20;
const HIGHLIGHT_NODE_COLOR = "#48CE73"; // green
const GAP = 0.5;

// TODO: actually make sure these make sense and add tests
const regexpLookup = {
  pyspy: /^(?<packageName>(.*\/)*)(?<filename>.*\.py+)(?<line_info>.*)$/,
  rbspy: /^(?<func>.+? - )?(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
  gospy: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
  default: /^(?<packageName>(.*\/)*)(?<filename>.*)(?<line_info>.*)$/,
};

class FlameGraphRenderer extends React.Component {
  constructor() {
    super();
    this.state = {
      highlightStyle: { display: "none" },
      tooltipStyle: { display: "none" },
      resetStyle: { visibility: "hidden" },
      sortBy: "self",
      sortByDirection: "desc",
      view: "both",
    };
    this.canvasRef = React.createRef();
    this.tooltipRef = React.createRef();
  }

  componentDidMount() {
    this.canvas = this.canvasRef.current;
    this.ctx = this.canvas.getContext("2d");
    this.topLevel = 0; // Todo: could be a constant
    this.selectedLevel = 0;
    this.rangeMin = 0;
    this.rangeMax = 1;
    this.query = "";

    window.addEventListener("resize", this.resizeHandler);
    window.addEventListener("focus", this.focusHandler);

    if (this.props.shortcut) {
      this.props.shortcut.registerShortcut(
        this.reset,
        ["escape"],
        "Reset",
        "Reset Flamegraph View"
      );
    }
    this.props.actions.fetchJSON(this.props.renderURL);
  }

  componentDidUpdate(prevProps) {
    if (prevProps.renderURL != this.props.renderURL) {
      this.props.actions.fetchJSON(this.props.renderURL);
    }
    if (
      this.props.flamebearer &&
      prevProps.flamebearer != this.props.flamebearer
    ) {
      this.updateData(this.props.flamebearer);
    }
  }

  rect(ctx, x, y, w, h, radius) {
    return ctx.rect(x, y, w, h);
  }

  roundRect(ctx, x, y, w, h, radius) {
    if (radius >= w / 2) {
      return this.rect(ctx, x, y, w, h, radius);
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

  updateZoom(i, j) {
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
    this.updateResetStyle();
  }

  updateData = () => {
    const { names, levels, numTicks, sampleRate } = this.props.flamebearer;
    this.names = names;
    this.levels = levels;
    this.numTicks = numTicks;
    this.sampleRate = sampleRate;
    this.renderCanvas();
  };

  // binary search of a block in a stack level
  binarySearchLevel(x, level, tickToX) {
    let i = 0;
    let j = level.length - 4;
    while (i <= j) {
      const m = 4 * ((i / 4 + j / 4) >> 1);
      const x0 = tickToX(level[m]);
      const x1 = tickToX(level[m] + level[m + 1]);
      if (x0 <= x && x1 >= x) {
        return x1 - x0 > COLLAPSE_THRESHOLD ? m : -1;
      }
      if (x0 > x) {
        j = m - 4;
      } else {
        i = m + 4;
      }
    }
    return -1;
  }

  getPackageNameFromStackTrace(spyName, stackTrace) {
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

  updateResetStyle = () => {
    // const emptyQuery = this.query === "";
    const topLevelSelected = this.selectedLevel === 0;
    this.setState({
      resetStyle: { visibility: topLevelSelected ? "hidden" : "visible" },
    });
  };

  handleSearchChange = (e) => {
    this.query = e.target.value;
    this.updateResetStyle();
    this.renderCanvas();
  };

  reset = () => {
    this.updateZoom(0, 0);
    this.renderCanvas();
  };

  xyToBar = (x, y) => {
    const i = Math.floor(y / PX_PER_LEVEL) + this.topLevel;
    if (i >= 0 && i < this.levels.length) {
      const j = this.binarySearchLevel(x, this.levels[i], this.tickToX);
      return { i, j };
    }
    return { i: 0, j: 0 };
  };

  clickHandler = (e) => {
    const { i, j } = this.xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);
    if (j === -1) return;

    this.updateZoom(i, j);
    this.renderCanvas();
    this.mouseOutHandler();
  };

  resizeHandler = () => {
    // this is here to debounce resize events (see: https://css-tricks.com/debouncing-throttling-explained-examples/)
    //   because rendering is expensive
    clearTimeout(this.resizeFinish);
    this.resizeFinish = setTimeout(this.renderCanvas, 100);
  };

  focusHandler = () => {
    this.renderCanvas();
  };

  tickToX = (i) => (i - this.numTicks * this.rangeMin) * this.pxPerTick;

  updateView = (newView) => {
    this.setState({
      view: newView,
    });
    // console.log('render-canvas');
    setTimeout(this.renderCanvas, 0);
  };

  renderCanvas = () => {
    if (!this.names) {
      return;
    }

    const { names, levels, numTicks, sampleRate } = this;
    this.graphWidth = this.canvas.width = this.canvas.clientWidth;
    this.pxPerTick =
      this.graphWidth / numTicks / (this.rangeMax - this.rangeMin);
    this.canvas.height = PX_PER_LEVEL * (levels.length - this.topLevel);
    this.canvas.style.height = `${this.canvas.height}px`;

    if (devicePixelRatio > 1) {
      this.canvas.width *= 2;
      this.canvas.height *= 2;
      this.ctx.scale(2, 2);
    }

    this.ctx.textBaseline = "middle";
    this.ctx.font =
      '400 12px system-ui, -apple-system, "Segoe UI", "Roboto", "Ubuntu", "Cantarell", "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"';

    const df = new DurationFormater(this.numTicks / this.sampleRate);
    // i = level
    for (let i = 0; i < levels.length - this.topLevel; i++) {
      const level = levels[this.topLevel + i];

      for (let j = 0; j < level.length; j += 4) {
        // j = 0: x start of bar
        // j = 1: width of bar
        // j = 2: position in the main index

        const barIndex = level[j];
        const x = this.tickToX(barIndex);
        const y = i * PX_PER_LEVEL;
        let numBarTicks = level[j + 1];

        // For this particular bar, there is a match
        const queryExists = this.query.length > 0;
        const nodeIsInQuery =
          (this.query && names[level[j + 3]].indexOf(this.query) >= 0) || false;
        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * this.pxPerTick <= COLLAPSE_THRESHOLD;

        // const collapsed = false;
        if (collapsed) {
          while (
            j < level.length - 3 &&
            barIndex + numBarTicks === level[j + 3] &&
            level[j + 4] * this.pxPerTick <= COLLAPSE_THRESHOLD &&
            nodeIsInQuery ===
              ((this.query && names[level[j + 5]].indexOf(this.query) >= 0) ||
                false)
          ) {
            j += 4;
            numBarTicks += level[j + 1];
          }
        }
        // ticks are samples
        const sw = numBarTicks * this.pxPerTick - (collapsed ? 0 : GAP);
        const sh = PX_PER_LEVEL - GAP;

        // if (x < -1 || x + sw > this.graphWidth + 1 || sw < HIDE_THRESHOLD) continue;

        this.ctx.beginPath();
        this.rect(this.ctx, x, y, sw, sh, 3);

        const ratio = numBarTicks / numTicks;

        const a = this.selectedLevel > i ? 0.33 : 1;

        const { spyName } = this.props.flamebearer;

        let nodeColor;
        if (collapsed) {
          nodeColor = colorGreyscale(200, 0.66);
        } else if (queryExists && nodeIsInQuery) {
          nodeColor = HIGHLIGHT_NODE_COLOR;
        } else if (queryExists && !nodeIsInQuery) {
          nodeColor = colorGreyscale(200, 0.66);
        } else {
          nodeColor = colorBasedOnPackageName(
            this.getPackageNameFromStackTrace(spyName, names[level[j + 3]]),
            a
          );
        }

        this.ctx.fillStyle = nodeColor;
        this.ctx.fill();

        if (!collapsed && sw >= LABEL_THRESHOLD) {
          const percent = formatPercent(ratio);
          const name = `${names[level[j + 3]]} (${percent}, ${df.format(
            numBarTicks / sampleRate
          )})`;

          this.ctx.save();
          this.ctx.clip();
          this.ctx.fillStyle = "black";
          this.ctx.fillText(name, Math.round(Math.max(x, 0) + 3), y + sh / 2);
          this.ctx.restore();
        }
      }
    }
  };

  mouseMoveHandler = (e) => {
    const { i, j } = this.xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);

    if (
      j === -1 ||
      e.nativeEvent.offsetX < 0 ||
      e.nativeEvent.offsetX > this.graphWidth
    ) {
      this.mouseOutHandler();
      return;
    }

    this.canvas.style.cursor = "pointer";

    const level = this.levels[i];
    const x = Math.max(this.tickToX(level[j]), 0);
    const y = (i - this.topLevel) * PX_PER_LEVEL;
    const sw = Math.min(
      this.tickToX(level[j] + level[j + 1]) - x,
      this.graphWidth
    );

    const tooltipEl = this.tooltipRef.current;
    const numBarTicks = level[j + 1];
    const percent = formatPercent(numBarTicks / this.numTicks);

    // a little hacky but this is here so that we can get tooltipWidth after text is updated.
    const tooltipTitle = this.names[level[j + 3]];
    tooltipEl.children[0].innerText = tooltipTitle;
    const tooltipWidth = tooltipEl.clientWidth;

    const df = new DurationFormater(this.numTicks / this.sampleRate);

    this.setState({
      highlightStyle: {
        display: "block",
        left: `${this.canvas.offsetLeft + x}px`,
        top: `${this.canvas.offsetTop + y}px`,
        width: `${sw}px`,
        height: `${PX_PER_LEVEL}px`,
      },
      tooltipStyle: {
        display: "block",
        left: `${
          Math.min(
            this.canvas.offsetLeft + e.nativeEvent.offsetX + 15 + tooltipWidth,
            this.canvas.offsetLeft + this.graphWidth
          ) - tooltipWidth
        }px`,
        top: `${this.canvas.offsetTop + e.nativeEvent.offsetY + 12}px`,
      },
      tooltipTitle,
      tooltipSubtitle: `${percent}, ${numberWithCommas(
        numBarTicks
      )} samples, ${df.format(numBarTicks / this.sampleRate)}`,
    });
  };

  mouseOutHandler = () => {
    this.canvas.style.cursor = "";
    this.setState({
      highlightStyle: {
        display: "none",
      },
      tooltipStyle: {
        display: "none",
      },
    });
  };

  updateSortBy = (newSortBy) => {
    let dir = this.state.sortByDirection;
    if (this.state.sortBy == newSortBy) {
      dir = dir == "asc" ? "desc" : "asc";
    } else {
      dir = "desc";
    }
    this.setState({
      sortBy: newSortBy,
      sortByDirection: dir,
    });
  };

  render = () => (
    <div className="canvas-renderer">
      <div className="canvas-container">
        <ProfilerHeader
          view={this.state.view}
          handleSearchChange={this.handleSearchChange}
          reset={this.reset}
          updateView={this.updateView}
          resetStyle={this.state.resetStyle}
        />
        <div className="flamegraph-container panes-wrapper">
          <div
            className={clsx("pane", { hidden: this.state.view === "icicle" })}
          >
            <ProfilerTable
              flamebearer={this.props.flamebearer}
              sortByDirection={this.state.sortByDirection}
              sortBy={this.state.sortBy}
              updateSortBy={this.updateSortBy}
              view={this.state.view}
            />
          </div>
          <div
            className={clsx("pane", { hidden: this.state.view === "table" })}
          >
            <canvas
              className="flamegraph-canvas"
              height="0"
              ref={this.canvasRef}
              onClick={this.clickHandler}
              onMouseMove={this.mouseMoveHandler}
              onMouseOut={this.mouseOutHandler}
            />
          </div>
        </div>
        <div
          className={clsx("no-data-message", {
            visible:
              this.props.flamebearer && this.props.flamebearer.numTicks === 0,
          })}
        >
          <span>
            No profiling data available for this application / time range.
          </span>
        </div>
      </div>
      <div className="flamegraph-highlight" style={this.state.highlightStyle} />
      <div
        className="flamegraph-tooltip"
        ref={this.tooltipRef}
        style={this.state.tooltipStyle}
      >
        <div className="flamegraph-tooltip-name">{this.state.tooltipTitle}</div>
        <div>{this.state.tooltipSubtitle}</div>
      </div>
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
  renderURL: buildRenderURL(state),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchJSON,
    },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(withShortcut(FlameGraphRenderer));
