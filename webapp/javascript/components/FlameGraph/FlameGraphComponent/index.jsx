/*

This component is based on code from flamebearer project
  https://github.com/mapbox/flamebearer

ISC License

Copyright (c) 2018, Mapbox

Permission to use, copy, modify, and/or distribute this software for any purpose
with or without fee is hereby granted, provided that the above copyright notice
and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.

*/

/* eslint-disable no-bitwise */
/* eslint-disable no-nested-ternary */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable react/no-unused-state */
/* eslint-disable no-restricted-syntax */
import React from 'react';
import clsx from 'clsx';
import {
  numberWithCommas,
  formatPercent,
  getPackageNameFromStackTrace,
  getFormatter,
} from './format';
import {
  colorBasedOnDiff,
  colorBasedOnPackageName,
  colorGreyscale,
  diffColorGreen,
  diffColorRed,
} from './color';
import { fitToCanvasRect } from '../../../util/fitMode';

const formatSingle = {
  format: 'single',
  jStep: 4,
  jName: 3,
  getBarOffset: (level, j) => level[j],
  getBarTotal: (level, j) => level[j + 1],
  getBarTotalDiff: (level, j) => 0,
  getBarSelf: (level, j) => level[j + 2],
  getBarSelfDiff: (level, j) => 0,
  getBarName: (level, j) => level[j + 3],
};

const formatDouble = {
  format: 'double',
  jStep: 7,
  jName: 6,
  getBarOffset: (level, j) => level[j] + level[j + 3],
  getBarTotal: (level, j) => level[j + 4] + level[j + 1],
  getBarTotalLeft: (level, j) => level[j + 1],
  getBarTotalRght: (level, j) => level[j + 4],
  getBarTotalDiff: (level, j) => level[j + 4] - level[j + 1],
  getBarSelf: (level, j) => level[j + 5] + level[j + 2],
  getBarSelfLeft: (level, j) => level[j + 2],
  getBarSelfRght: (level, j) => level[j + 5],
  getBarSelfDiff: (level, j) => level[j + 5] - level[j + 2],
  getBarName: (level, j) => level[j + 6],
};

export function deltaDiff(levels, start, step) {
  for (const level of levels) {
    let prev = 0;
    for (let i = start; i < level.length; i += step) {
      level[i] += prev;
      prev = level[i] + level[i + 1];
    }
  }
}

export function deltaDiffWrapper(format, levels) {
  if (format === 'double') {
    deltaDiff(levels, 0, 7);
    deltaDiff(levels, 3, 7);
  } else {
    deltaDiff(levels, 0, 4);
  }
}

export function parseFlamebearerFormat(format) {
  const isSingle = format !== 'double';
  if (isSingle) return formatSingle;
  return formatDouble;
}

const PX_PER_LEVEL = 18;
const COLLAPSE_THRESHOLD = 5;
const LABEL_THRESHOLD = 20;
const HIGHLIGHT_NODE_COLOR = '#48CE73'; // green
const GAP = 0.5;
export const BAR_HEIGHT = PX_PER_LEVEL - GAP;

const unitsToFlamegraphTitle = {
  objects: 'amount of objects in RAM per function',
  bytes: 'amount of RAM per function',
  samples: 'CPU time per function',
};

const diffLegend = [
  100,
  50,
  20,
  10,
  5,
  3,
  2,
  1,
  0,
  -1,
  -2,
  -3,
  -5,
  -10,
  -20,
  -50,
  -100,
];

const rect = (ctx, x, y, w, h) => ctx.rect(x, y, w, h);

class FlameGraph extends React.Component {
  constructor(props) {
    super();
    this.state = {
      highlightStyle: { display: 'none' },
      tooltipStyle: { display: 'none' },
      resetStyle: { visibility: 'hidden' },
      sortBy: 'self',
      sortByDirection: 'desc',
      viewDiff: props.viewType === 'diff' ? 'diff' : undefined,
      flamebearer: null,
    };
    this.canvasRef = React.createRef();
    this.highlightRef = React.createRef();
    this.tooltipRef = React.createRef();
    this.currentJSONController = null;
  }

  componentDidMount() {
    this.canvas = this.canvasRef.current;
    this.ctx = this.canvas.getContext('2d');
    this.topLevel = 0; // Todo: could be a constant
    this.selectedLevel = 0;
    this.rangeMin = 0;
    this.rangeMax = 1;

    window.addEventListener('resize', this.resizeHandler);
    window.addEventListener('focus', this.focusHandler);

    if (this.props.shortcut) {
      this.props.shortcut.registerShortcut(
        this.reset,
        ['escape'],
        'Reset',
        'Reset Flamegraph View'
      );
    }
    this.updateData();
  }

  componentDidUpdate(prevProps) {
    if (
      (this.props.flamebearer &&
        prevProps.flamebearer !== this.props.flamebearer) ||
      this.props.width !== prevProps.width ||
      this.props.height !== prevProps.height ||
      this.props.view !== prevProps.view ||
      this.props.fitMode !== prevProps.fitMode
    ) {
      this.updateData();
    }
    if (
      this.props.fitMode !== prevProps.fitMode ||
      this.props.query !== prevProps.query
    ) {
      setTimeout(() => this.renderCanvas(), 0);
    }
  }

  updateData = () => {
    const {
      names,
      levels,
      numTicks,
      sampleRate,
      units,
      format,
    } = this.props.flamebearer;
    this.setState(
      {
        names,
        levels,
        numTicks,
        sampleRate,
        units,
        format, // "single" | "double"
      },
      () => {
        this.renderCanvas();
      }
    );
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

  updateResetStyle = () => {
    const topLevelSelected = this.selectedLevel === 0;
    this.setState({
      resetStyle: { visibility: topLevelSelected ? 'hidden' : 'visible' },
    });
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

  createFormatter = () =>
    getFormatter(this.state.numTicks, this.state.sampleRate, this.state.units);

  renderCanvas = () => {
    if (
      !this.props.flamebearer ||
      !this.props.flamebearer.names ||
      this.props.flamebearer.names.length <= 1
    ) {
      return;
    }

    const { names, levels, numTicks, sampleRate } = this.props.flamebearer;
    const ff = this.props.format;
    const isDiff = this.props.viewType === 'diff';
    this.canvas.width = this.props.width || this.canvas.clientWidth;
    this.graphWidth = this.canvas.width;
    this.pxPerTick =
      this.graphWidth / numTicks / (this.rangeMax - this.rangeMin);
    this.canvas.height = this.props.height
      ? this.props.height - 30
      : PX_PER_LEVEL * (levels.length - this.topLevel);
    this.canvas.style.height = `${this.canvas.height}px`;
    this.canvas.style.cursor = 'pointer';

    if (devicePixelRatio > 1) {
      this.canvas.width *= 2;
      this.canvas.height *= 2;
      this.ctx.scale(2, 2);
    }

    this.ctx.textBaseline = 'middle';
    this.ctx.font =
      '400 11.5px SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace';
    // Since this is a monospaced font
    // any character would do
    const characterSize = this.ctx.measureText('a').width;
    this.formatter = this.createFormatter();
    // i = level
    for (let i = 0; i < levels.length - this.topLevel; i += 1) {
      const level = levels[this.topLevel + i];
      for (let j = 0; j < level.length; j += ff.jStep) {
        const barIndex = ff.getBarOffset(level, j);
        const x = this.tickToX(barIndex);
        const y = i * PX_PER_LEVEL;
        let numBarTicks = ff.getBarTotal(level, j);

        // For this particular bar, there is a match
        const queryExists = this.props.query.length > 0;
        const nodeIsInQuery =
          (this.props.query &&
            names[level[j + ff.jName]].indexOf(this.props.query) >= 0) ||
          false;
        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * this.pxPerTick <= COLLAPSE_THRESHOLD;
        const numBarDiff = collapsed ? 0 : ff.getBarTotalDiff(level, j);

        // const collapsed = false;
        if (collapsed) {
          // TODO: fix collapsed code
          while (
            j < level.length - ff.jStep &&
            barIndex + numBarTicks === ff.getBarOffset(level, j + ff.jStep) &&
            ff.getBarTotal(level, j + ff.jStep) * this.pxPerTick <=
              COLLAPSE_THRESHOLD &&
            nodeIsInQuery ===
              ((this.props.query &&
                names[level[j + ff.jStep + ff.jName]].indexOf(
                  this.props.query
                ) >= 0) ||
                false)
          ) {
            j += ff.jStep;
            numBarTicks += ff.getBarTotal(level, j);
          }
        }
        // ticks are samples
        const sw = numBarTicks * this.pxPerTick - (collapsed ? 0 : GAP);
        const sh = BAR_HEIGHT;

        // if (x < -1 || x + sw > this.graphWidth + 1 || sw < HIDE_THRESHOLD) continue;

        this.ctx.beginPath();
        rect(this.ctx, x, y, sw, sh, 3);

        const ratio = numBarTicks / numTicks;

        const a = this.selectedLevel > i ? 0.33 : 1;

        const { spyName } = this.props.flamebearer;

        let nodeColor;
        if (isDiff && collapsed) {
          nodeColor = colorGreyscale(200, 0.66);
        } else if (isDiff) {
          nodeColor = colorBasedOnDiff(
            numBarDiff,
            ff.getBarTotalLeft(level, j),
            a
          );
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
          const longName = `${shortName} (${percent}, ${this.formatter.format(
            numBarTicks,
            sampleRate
          )})`;

          const namePosX = Math.round(Math.max(x, 0));
          const fitCalc = fitToCanvasRect({
            mode: this.props.fitMode,
            charSize: characterSize,
            rectWidth: sw,
            fullText: longName,
            shortText: shortName,
          });

          this.ctx.save();
          this.ctx.clip();
          this.ctx.fillStyle = 'black';
          // when showing the code, give it a space in the beginning
          this.ctx.fillText(
            fitCalc.text,
            namePosX + fitCalc.marginLeft,
            y + sh / 2 + 1
          );
          this.ctx.restore();
        }
      }
    }
  };

  mouseMoveHandler = (e) => {
    const ff = this.props.format;
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

    let tooltipText;
    let tooltipDiffText = '';
    let tooltipDiffColor = '';
    if (ff.format !== 'double') {
      tooltipText = `${percent}, ${numberWithCommas(
        numBarTicks
      )} samples, ${this.formatter.format(numBarTicks, this.state.sampleRate)}`;
    } else {
      const totalLeft = ff.getBarTotalLeft(level, j);
      const totalRght = ff.getBarTotalRght(level, j);
      const totalDiff = ff.getBarTotalDiff(level, j);
      tooltipText = `Left: ${numberWithCommas(
        totalLeft
      )} samples, ${this.formatter.format(totalLeft, this.state.sampleRate)}`;
      tooltipText += `\nRight: ${numberWithCommas(
        totalRght
      )} samples, ${this.formatter.format(totalRght, this.state.sampleRate)}`;
      tooltipDiffColor =
        totalDiff === 0 ? '' : totalDiff > 0 ? diffColorRed : diffColorGreen;
      tooltipDiffText = !totalLeft
        ? ' (new)'
        : !totalRght
        ? ' (removed)'
        : ` (${totalDiff > 0 ? '+' : ''}${formatPercent(
            totalDiff / totalLeft
          )})`;
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

    tooltipEl.children[0].innerText = tooltipTitle;
    tooltipEl.children[1].children[0].innerText = tooltipText;
    tooltipEl.children[1].children[1].innerText = tooltipDiffText;
    tooltipEl.children[1].children[1].style.color = tooltipDiffColor;

    // makes it so that tooltip is always visible even if mouse is close to the right edge
    const tooltipX = Math.min(
      e.clientX + 12,
      window.innerWidth - tooltipEl.clientWidth - 20
    );
    tooltipEl.style.left = `${tooltipX}px`;
    tooltipEl.style.top = `${e.clientY + 20}px`;
  };

  mouseOutHandler = () => {
    this.highlightRef.current.style.opacity = '0';
    this.tooltipRef.current.style.opacity = '0';
  };

  updateZoom(i, j) {
    const ff = this.props.format;
    if (!Number.isNaN(i) && !Number.isNaN(j)) {
      this.selectedLevel = i;
      this.topLevel = 0;
      this.rangeMin =
        ff.getBarOffset(this.state.levels[i], j) / this.state.numTicks;
      this.rangeMax =
        (ff.getBarOffset(this.state.levels[i], j) +
          ff.getBarTotal(this.state.levels[i], j)) /
        this.state.numTicks;
    } else {
      this.selectedLevel = 0;
      this.topLevel = 0;
      this.rangeMin = 0;
      this.rangeMax = 1;
    }

    this.props.onZoom(this.selectedLevel);
  }

  // binary search of a block in a stack level
  binarySearchLevel(x, level, tickToX) {
    const ff = this.props.format;

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

  render = () => {
    const { ExportData } = this.props;
    const dataUnavailable =
      !this.props.flamebearer ||
      (this.props.flamebearer && this.props.flamebearer.names.length <= 1);

    return (
      <>
        <div
          key="flamegraph-pane"
          data-testid="flamegraph-view"
          className={clsx('flamegraph-pane', {
            'vertical-orientation': this.props.viewType === 'double',
          })}
        >
          <div className="flamegraph-header">
            {!this.state.viewDiff ? (
              <div>
                <div className="row flamegraph-title">
                  Frame width represents{' '}
                  {unitsToFlamegraphTitle[this.state.units]}
                </div>
              </div>
            ) : (
              <div>
                <div className="row">
                  Base graph: left - Comparison graph: right
                </div>
                <div className="row flamegraph-legend">
                  <div className="flamegraph-legend-list">
                    {diffLegend.map((v) => (
                      <div
                        key={v}
                        className="flamegraph-legend-item"
                        style={{
                          backgroundColor: colorBasedOnDiff(v, 100, 0.8),
                        }}
                      >
                        {v > 0 ? '+' : ''}
                        {v}%
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}
            {ExportData && !dataUnavailable ? (
              <ExportData
                flameCanvas={this.canvasRef}
                label={this.props.label || ''}
              />
            ) : null}
          </div>
          {dataUnavailable ? (
            <div className="error-message">
              <span>
                No profiling data available for this application / time range.
              </span>
            </div>
          ) : null}
          <div
            style={{
              opacity: dataUnavailable ? 0 : 1,
            }}
          >
            <canvas
              className="flamegraph-canvas"
              height="0"
              data-testid="flamegraph-canvas"
              data-appname={this.props.label}
              ref={this.canvasRef}
              onClick={this.clickHandler}
              onMouseMove={this.mouseMoveHandler}
              onMouseOut={this.mouseOutHandler}
              onBlur={() => {}}
            />
            <div className="flamegraph-highlight" ref={this.highlightRef} />
          </div>
        </div>
        <div className="flamegraph-tooltip" ref={this.tooltipRef}>
          <div className="flamegraph-tooltip-name" />
          <div>
            <span />
            <span />
          </div>
        </div>
      </>
    );
  };
}

export default FlameGraph;
