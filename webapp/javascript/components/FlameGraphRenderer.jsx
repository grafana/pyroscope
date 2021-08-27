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

import React, { Fragment } from "react";
import { connect } from "react-redux";

import clsx from "clsx";

import { bindActionCreators } from "redux";

import { withShortcut } from "react-keybind";

import { buildDiffRenderURL, buildRenderURL } from "../util/updateRequests";
import {
  numberWithCommas,
  formatPercent,
  getPackageNameFromStackTrace,
  getFormatter,
} from "../util/format";
import { fitToCanvasRect } from "../util/fitMode.js";
import { colorBasedOnDiff, colorBasedOnPackageName, colorGreyscale, diffColorGreen, diffColorRed } from "../util/color";
import TimelineChartWrapper from "./TimelineChartWrapper";
import ProfilerTable from "./ProfilerTable";
import ProfilerHeader from "./ProfilerHeader";
import { deltaDiffWrapper, parseFlamebearerFormat } from "../util/flamebearer";

import ExportData from "./ExportData";


const PX_PER_LEVEL = 18;
const COLLAPSE_THRESHOLD = 5;
const LABEL_THRESHOLD = 20;
const HIGHLIGHT_NODE_COLOR = "#48CE73"; // green
const GAP = 0.5;

const unitsToFlamegraphTitle = {
  "objects": "amount of objects in RAM per function",
  "bytes": "amount of RAM per function",
  "samples": "CPU time per function",
}

const diffLegend = [100, 50, 20, 10, 5, 3, 2, 1, 0, -1, -2, -3, -5, -10, -20, -50, -100];

class FlameGraphRenderer extends React.Component {
  constructor(props) {
    super();
    this.state = {
      highlightStyle: { display: "none" },
      tooltipStyle: { display: "none" },
      resetStyle: { visibility: "hidden" },
      sortBy: "self",
      sortByDirection: "desc",
      view: "both",
      viewDiff: props.viewType === "diff" ? "diff" : undefined,
      flamebearer: null,
      fitMode: props.fitMode ? props.fitMode : "HEAD",
    };
    this.canvasRef = React.createRef();
    this.highlightRef = React.createRef();
    this.tooltipRef = React.createRef();
    this.currentJSONController = null;
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

    if(this.props.viewSide === 'left' || this.props.viewSide === 'right') {
      this.fetchFlameBearerData(this.props[`${this.props.viewSide}RenderURL`])
    } else if (this.props.viewType === 'single') {
      this.fetchFlameBearerData(this.props.renderURL)
    } else if (this.props.viewType === 'diff') {
      this.fetchFlameBearerData(this.props.diffRenderURL);
    }
  }

  componentDidUpdate(prevProps, prevState) {
    const propsChanged = this.getParamsFromRenderURL(this.props.renderURL).query != this.getParamsFromRenderURL(prevProps.renderURL).query ||
      prevProps.maxNodes != this.props.maxNodes ||
      prevProps.refreshToken != this.props.refreshToken;

    if (propsChanged ||
      prevProps.from != this.props.from ||
      prevProps.until != this.props.until ||
      prevProps[`${this.props.viewSide}From`] != this.props[`${this.props.viewSide}From`] ||
      prevProps[`${this.props.viewSide}Until`] != this.props[`${this.props.viewSide}Until`]
    ) {
      if(this.props.viewSide === 'left' || this.props.viewSide === 'right') {
        this.fetchFlameBearerData(this.props[`${this.props.viewSide}RenderURL`])
      } else if (this.props.viewType === 'single') {
        this.fetchFlameBearerData(this.props.renderURL)
      }
    }

    if (this.props.viewType === 'diff') {
      if (propsChanged
        || prevProps.leftFrom != this.props.leftFrom || prevProps.leftUntil != this.props.leftUntil
        || prevProps.rightFrom != this.props.rightFrom || prevProps.rightUntil != this.props.rightUntil
      ) {
        this.fetchFlameBearerData(this.props.diffRenderURL);
      }
    }

    if (
      this.state.flamebearer &&
      prevState.flamebearer != this.state.flamebearer
    ) {
      this.updateData();
    }
  }

  fetchFlameBearerData(url) {
    if (this.currentJSONController) {
      this.currentJSONController.abort();
    }
    this.currentJSONController = new AbortController();

    fetch(`${url}&format=json`, { signal: this.currentJSONController.signal })
      .then((response) => response.json())
      .then((data) => {
        let { flamebearer } = data;
        deltaDiffWrapper(flamebearer.format, flamebearer.levels);

        this.setState({
          flamebearer: flamebearer,
        }, () => {
          this.updateData();
        })
      })
      .finally();
  }

  getParamsFromRenderURL(inputURL) {
    let urlParamsRegexp = /(.*render\?)(?<urlParams>(.*))/
    let paramsString = inputURL.match(urlParamsRegexp);

    let params = new URLSearchParams(paramsString.groups.urlParams);
    let paramsObj = this.paramsToObject(params);

    return paramsObj
  }

  paramsToObject(entries) {
    const result = {}
    for(const [key, value] of entries) { // each 'entry' is a [key, value] tupple
      result[key] = value;
    }
    return result;
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
    const ff = this.parseFormat();
    if (!Number.isNaN(i) && !Number.isNaN(j)) {
      this.selectedLevel = i;
      this.topLevel = 0;
      this.rangeMin = ff.getBarOffset(this.state.levels[i], j) / this.state.numTicks;
      this.rangeMax =
        (ff.getBarOffset(this.state.levels[i], j) + ff.getBarTotal(this.state.levels[i], j)) / this.state.numTicks;
    } else {
      this.selectedLevel = 0;
      this.topLevel = 0;
      this.rangeMin = 0;
      this.rangeMax = 1;
    }
    this.updateResetStyle();
  }

  updateData = () => {
    const { names, levels, numTicks, sampleRate, units, format } = this.state.flamebearer;
    this.setState({
      names: names,
      levels: levels,
      numTicks: numTicks,
      sampleRate: sampleRate,
      units: units,
      format: format, // "single" | "double"
    }, () => {
      this.renderCanvas();
    });
  };

  // format=single
  //   j = 0: x start of bar
  //   j = 1: width of bar
  //   j = 3: position in the main index (jStep)
  //
  // format=double
  //   j = 0,3: x start of bar =>     x = (level[0] + level[3]) / 2
  //   j = 1,4: width of bar   => width = (level[1] + level[4]) / 2
  //                           =>  diff = (level[4] - level[1]) / (level[1] + level[4])
  //   j = 6  : position in the main index (jStep)
  parseFormat(format) {
    return parseFlamebearerFormat(format || this.state.format);
  }

  // binary search of a block in a stack level
  binarySearchLevel(x, level, tickToX) {
    const ff = this.parseFormat();

    let i = 0;
    let j = level.length - ff.jStep;
    while (i <= j) {
      const m = ff.jStep * ((i / ff.jStep + j / ff.jStep) >> 1);
      const x0 = tickToX(ff.getBarOffset(level, m));
      const x1 = tickToX(ff.getBarOffset(level, m) + ff.getBarTotal(level, m));
      if (x0 <= x && x1 >= x) {
        return x1 - x0 > COLLAPSE_THRESHOLD ? m : -1;
      }
      if (x0 > x) {
        j = m - ff.jStep;
      } else {
        i = m + ff.jStep;
      }
    }
    return -1;
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
    if (i >= 0 && i < this.state.levels.length) {
      const j = this.binarySearchLevel(x, this.state.levels[i], this.tickToX);
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

  tickToX = (i) => (i - this.state.numTicks * this.rangeMin) * this.pxPerTick;

  updateView = (newView) => {
    this.setState({
      view: newView,
    });
    setTimeout(this.renderCanvas, 0);
  };

  updateViewDiff = (newView) => {
    this.setState({
      viewDiff: newView,
    });
    setTimeout(this.renderCanvas, 0);
  };

  updateFitMode = (newFitMode) => {
    this.setState({
      fitMode: newFitMode,
    });
    setTimeout(this.renderCanvas, 0);
  }

  createFormatter = () => {
    return getFormatter(this.state.numTicks, this.state.sampleRate, this.state.units);
  }

  renderCanvas = () => {
    if (!this.state.names) {
      return;
    }

    const { names, levels, numTicks, sampleRate, units, fitMode } = this.state;
    const ff = this.parseFormat();
    const isDiff = this.props.viewType === "diff";

    this.graphWidth = this.canvas.width = this.canvas.clientWidth;
    this.pxPerTick =
      this.graphWidth / numTicks / (this.rangeMax - this.rangeMin);
    this.canvas.height = PX_PER_LEVEL * (levels.length - this.topLevel);
    this.canvas.style.height = `${this.canvas.height}px`;
    this.canvas.style.cursor = "pointer";

    if (devicePixelRatio > 1) {
      this.canvas.width *= 2;
      this.canvas.height *= 2;
      this.ctx.scale(2, 2);
    }

    this.ctx.textBaseline = "middle";
    this.ctx.font = '400 11.5px SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace';
    // Since this is a monospaced font
    // any character would do
    const characterSize = this.ctx.measureText("a").width;

    this.formatter = this.createFormatter();
    // i = level
    for (let i = 0; i < levels.length - this.topLevel; i++) {
      const level = levels[this.topLevel + i];
      for (let j = 0; j < level.length; j += ff.jStep) {

        const barIndex = ff.getBarOffset(level, j);
        const x = this.tickToX(barIndex);
        const y = i * PX_PER_LEVEL;
        let numBarTicks = ff.getBarTotal(level, j);

        // For this particular bar, there is a match
        const queryExists = this.query.length > 0;
        const nodeIsInQuery =
          (this.query && names[level[j + ff.jName]].indexOf(this.query) >= 0) || false;
        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * this.pxPerTick <= COLLAPSE_THRESHOLD;
        const numBarDiff = collapsed ? 0 : ff.getBarTotalDiff(level, j);

        // const collapsed = false;
        if (collapsed) { // TODO: fix collapsed code
          while (
            j < level.length - ff.jStep &&
            barIndex + numBarTicks === ff.getBarOffset(level, j + ff.jStep) &&
            ff.getBarTotal(level, j + ff.jStep) * this.pxPerTick <= COLLAPSE_THRESHOLD &&
            nodeIsInQuery === ((this.query && names[level[j + ff.jStep + ff.jName]].indexOf(this.query) >= 0) || false)
          ) {
            j += ff.jStep;
            numBarTicks += ff.getBarTotal(level, j);
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

        const { spyName } = this.state.flamebearer;

        let nodeColor;
        if (isDiff && collapsed) {
          nodeColor = colorGreyscale(200, 0.66);
        } else if (isDiff) {
          nodeColor = colorBasedOnDiff(numBarDiff, ff.getBarTotalLeft(level, j), a);
        } else if (collapsed) {
          nodeColor = colorGreyscale(200, 0.66);
        } else if (queryExists && nodeIsInQuery) {
          nodeColor = HIGHLIGHT_NODE_COLOR;
        } else if (queryExists && !nodeIsInQuery) {
          nodeColor = colorGreyscale(200, 0.66);
        } else {
          nodeColor = colorBasedOnPackageName(
            getPackageNameFromStackTrace(spyName, names[level[j + ff.jName]]),
            a
          );
        }

        this.ctx.fillStyle = nodeColor;
        this.ctx.fill();

        if (!collapsed && sw >= LABEL_THRESHOLD) {
          const percent = formatPercent(ratio);
          const shortName = names[level[j + ff.jName]];
          const longName = `${shortName} (${percent}, ${this.formatter.format(numBarTicks, sampleRate)})`

          let namePosX = Math.round(Math.max(x, 0));
          let fitCalc = fitToCanvasRect({
            mode: fitMode,
            charSize: characterSize,
            rectWidth: sw,
            fullText: longName,
            shortText: shortName,
          });

          this.ctx.save();
          this.ctx.clip();
          this.ctx.fillStyle = "black";
          // when showing the code, give it a space in the beginning
          this.ctx.fillText(fitCalc.text, namePosX + fitCalc.marginLeft, y + sh / 2+1);
          this.ctx.restore();
        }
      }
    }
  };

  mouseMoveHandler = (e) => {
    const ff = this.parseFormat();
    const { i, j } = this.xyToBar(e.nativeEvent.offsetX, e.nativeEvent.offsetY);

    if (
      j === -1 ||
      e.nativeEvent.offsetX < 0 ||
      e.nativeEvent.offsetX > this.graphWidth
    ) {
      this.mouseOutHandler();
      return;
    }

    const level = this.state.levels[i];
    const x = Math.max(this.tickToX(ff.getBarOffset(level, j)), 0);
    const y = (i - this.topLevel) * PX_PER_LEVEL;
    const sw = Math.min(
      this.tickToX(ff.getBarOffset(level, j) + ff.getBarTotal(level, j)) - x,
      this.graphWidth
    );

    const highlightEl = this.highlightRef.current;
    const tooltipEl = this.tooltipRef.current;
    const numBarTicks = ff.getBarTotal(level, j);
    const percent = formatPercent(numBarTicks / this.state.numTicks);
    const tooltipTitle = this.state.names[level[j + ff.jName]];

    var tooltipText, tooltipDiffText = '', tooltipDiffColor = ''
    if (ff.format !== "double") {
      tooltipText = `${percent}, ${numberWithCommas(numBarTicks)} samples, ${this.formatter.format(numBarTicks, this.state.sampleRate)}`;
    } else {
      const totalLeft = ff.getBarTotalLeft(level, j);
      const totalRght = ff.getBarTotalRght(level, j);
      const totalDiff = ff.getBarTotalDiff(level, j);
      tooltipText  = `Left: ${numberWithCommas(totalLeft)} samples, ${this.formatter.format(totalLeft, this.state.sampleRate)}`;
      tooltipText += `\nRight: ${numberWithCommas(totalRght)} samples, ${this.formatter.format(totalRght, this.state.sampleRate)}`;
      tooltipDiffColor = totalDiff === 0 ? '' : totalDiff > 0 ? diffColorRed : diffColorGreen;
      tooltipDiffText = !totalLeft ? ' (new)'
                      : !totalRght ? ' (removed)'
                      : ' (' + (totalDiff > 0 ? '+' : '') + formatPercent(totalDiff / totalLeft) + ')';
    }

    // Before you change all of this to React consider performance implications.
    // Doing this with setState leads to significant lag.
    // See this issue https://github.com/pyroscope-io/pyroscope/issues/205
    //   and this PR https://github.com/pyroscope-io/pyroscope/pull/266 for more info.
    highlightEl.style.opacity = 1;
    highlightEl.style.left = `${this.canvas.offsetLeft + x}px`;
    highlightEl.style.top = `${this.canvas.offsetTop + y}px`;
    highlightEl.style.width = `${sw}px`;
    highlightEl.style.height = `${PX_PER_LEVEL}px`;

    tooltipEl.style.opacity = 1;
    tooltipEl.style.left = `${e.clientX+12}px`;
    tooltipEl.style.top = `${e.clientY+12}px`;

    tooltipEl.children[0].innerText = tooltipTitle;
    tooltipEl.children[1].children[0].innerText = tooltipText;
    tooltipEl.children[1].children[1].innerText = tooltipDiffText;
    tooltipEl.children[1].children[1].style.color = tooltipDiffColor;
  };

  mouseOutHandler = () => {
    this.highlightRef.current.style.opacity = "0";
    this.tooltipRef.current.style.opacity = "0";
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

  render = () => {
    // This is necessary because the order switches depending on single vs comparison view
    let tablePane = (
      <div
        key={'table-pane'}
        className={clsx("pane", { hidden: this.state.view === "icicle", "vertical-orientation": this.props.viewType === "double" })}
      >
        <ProfilerTable
          flamebearer={this.state.flamebearer}
          sortByDirection={this.state.sortByDirection}
          sortBy={this.state.sortBy}
          updateSortBy={this.updateSortBy}
          view={this.state.view}
          viewDiff={this.state.viewDiff}
          fitMode={this.state.fitMode}
        />
      </div>
    )

    let flameGraphPane = (
      <div
        key={'flamegraph-pane'}
        className={clsx("pane", { hidden: this.state.view === "table", "vertical-orientation": this.props.viewType === "double" })}
      >
        <div className='flamegraph-header'>
          <span></span>
          { !this.state.viewDiff ?
            <div>
              <div className="row">
                Frame width represents {unitsToFlamegraphTitle[this.state.units]}
              </div>
            </div> :
            <div>
              <div className="row">
                Base graph: left - Comparison graph: right
              </div>
              <div className="row flamegraph-legend">
                <div className="flamegraph-legend-list">
                  {diffLegend.map((v) => (
                    <div key={v} className="flamegraph-legend-item" style={{ backgroundColor: colorBasedOnDiff(v, 100, 0.8) }}>
                      {v > 0 ? '+' : ''}{v}%
                    </div>
                  ))}
                </div>
              </div>
            </div>
          }
          <ExportData flameCanvas={this.canvasRef}/>
        </div>
        <canvas
          className="flamegraph-canvas"
          height="0"
          ref={this.canvasRef}
          onClick={this.clickHandler}
          onMouseMove={this.mouseMoveHandler}
          onMouseOut={this.mouseOutHandler}
        />
      </div>
    )

    let panes = this.props.viewType === "double" ?
      [flameGraphPane, tablePane]:
      [tablePane, flameGraphPane]

    const flotData = this.props.timeline
      ? [this.props.timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
      : [];

    return (
      <div className={clsx("canvas-renderer", { "double": this.props.viewType === "double" })}>

        <div className="canvas-container">
          <ProfilerHeader
            view={this.state.view}
            viewDiff={this.state.viewDiff}
            handleSearchChange={this.handleSearchChange}
            reset={this.reset}
            updateView={this.updateView}
            updateViewDiff={this.updateViewDiff}
            resetStyle={this.state.resetStyle}
            updateFitMode={this.updateFitMode}
            fitMode={this.state.fitMode}
          />
          {
            this.props.viewType === "double"
              ? <Fragment>
                <InstructionText {...this.props}/>
                <TimelineChartWrapper
                  key={`timeline-chart-${this.props.viewSide}`}
                  id={`timeline-chart-${this.props.viewSide}`}
                  viewSide={this.props.viewSide}
                />
              </Fragment>
              : this.props.viewType === "diff"
              ? <div className="diff-instructions-wrapper">
                <div className="diff-instructions-wrapper-side">
                  <InstructionText {...this.props} viewSide="left"/>
                  <TimelineChartWrapper
                    key={`timeline-chart-left`}
                    id={`timeline-chart-left`}
                    viewSide="left"
                  />
                </div>
                <div className="diff-instructions-wrapper-side">
                  <InstructionText {...this.props} viewSide="right"/>
                  <TimelineChartWrapper
                    key={`timeline-chart-right`}
                    id={`timeline-chart-right`}
                    viewSide="right"
                  />
                </div>
              </div>
              : null
          }
          <div className={clsx("flamegraph-container panes-wrapper", { "vertical-orientation": this.props.viewType === "double" })}>
            {
              panes.map((pane) => (
                pane
              ))
            }
            {/* { tablePane }
            { flameGraphPane } */}
          </div>
          <div
            className={clsx("no-data-message", {
              visible:
                this.state.flamebearer && this.state.flamebearer.numTicks === 0,
            })}
          >
            <span>
              No profiling data available for this application / time range.
            </span>
          </div>
        </div>
        <div
          className="flamegraph-highlight"
          ref={this.highlightRef}
        />
        <div
          className="flamegraph-tooltip"
          ref={this.tooltipRef}
        >
          <div className="flamegraph-tooltip-name"></div>
          <div><span></span><span></span></div>
        </div>
      </div>
    )
  }
}

function InstructionText(props) {
  const {viewType, viewSide} = props;
  let instructionsText = viewType === "double" || viewType === "diff" ? `Select ${viewSide} time range` : null;
  let instructionsClassName = viewType === "double" || viewType === "diff" ? `${viewSide}-instructions` : null;

  return (
    <div className={`${instructionsClassName}-wrapper`}>
      <span className={`${instructionsClassName}-text`}>{instructionsText}</span>
    </div>
  )
}

const mapStateToProps = (state) => ({
  ...state,
  renderURL: buildRenderURL(state),
  leftRenderURL: buildRenderURL(state, state.leftFrom, state.leftUntil),
  rightRenderURL: buildRenderURL(state, state.rightFrom, state.rightUntil),
  diffRenderURL: buildDiffRenderURL(state, state.leftFrom, state.leftUntil, state.rightFrom, state.rightUntil),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    { },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(withShortcut(FlameGraphRenderer));
