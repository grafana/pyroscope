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
import { MenuItem } from '@szhsin/react-menu';
import {
  getFormatter,
  ratioToPercent,
  formatPercent,
} from '../../../util/format';
import DiffLegend from './DiffLegend';
import Tooltip from './Tooltip';
import Highlight from './Highlight';
import ContextMenu from './ContextMenu';
import { PX_PER_LEVEL, COLLAPSE_THRESHOLD } from './constants';
import { RenderCanvas } from './CanvasRenderer';
import { getRatios } from './utils';

const unitsToFlamegraphTitle = {
  objects: 'amount of objects in RAM per function',
  bytes: 'amount of RAM per function',
  samples: 'CPU time per function',
};

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
    this.canvasRef2 = React.createRef();
    this.highlightRef = React.createRef();
    this.tooltipRef = React.createRef();
    this.currentJSONController = null;
  }

  componentDidMount() {
    this.canvas = this.canvasRef.current;

    this.ctx = this.canvas.getContext('2d');
    this.topLevel = 0; // Todo: could be a constant
    this.rangeMin = 0;
    this.rangeMax = 1;

    // TODO(eh-am): rename this?
    // selected = when you left click
    // focused = when you right click and "Focus"
    this.selectedLevel = 0;
    this.focusedNode = null;

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

    console.log('this.props.format', this.props.format);
    console.log('this.props.flamebearer.format', this.props.flamebearer.format);
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
    const { names, levels, numTicks, sampleRate, units, format } =
      this.props.flamebearer;
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
    RenderCanvas({
      canvas: this.canvas,
      viewType: this.props.flamebearer.format,

      numTicks: this.props.flamebearer.numTicks,
      sampleRate: this.props.flamebearer.sampleRate,
      names: this.props.flamebearer.names,
      levels: this.props.flamebearer.levels,
      topLevel: this.topLevel,

      rangeMin: this.rangeMin,
      rangeMax: this.rangeMax,

      units: this.state.units,
      fitMode: this.props.fitMode,

      selectedLevel: this.selectedLevel,

      leftTicks: this.props.flamebearer.leftTicks,
      rightTicks: this.props.flamebearer.rightTicks,
    });

    this.graphWidth = this.canvas.width;
    this.pxPerTick =
      this.graphWidth /
      this.props.flamebearer.numTicks /
      (this.rangeMax - this.rangeMin);
  };

  // TODO(eh-am): need a better name
  xyToTooltipData = (format, x, y) => {
    const ff = this.props.format;
    const { i, j } = this.xyToBar(x, y);

    const level = this.state.levels[i];
    const title = this.state.names[level[j + ff.jName]];

    switch (format) {
      case 'single': {
        const numBarTicks = ff.getBarTotal(level, j);
        const percent = formatPercent(numBarTicks / this.state.numTicks);

        return {
          format: 'single',
          title,
          numBarTicks,
          percent,
        };
      }

      case 'double': {
        const totalLeft = ff.getBarTotalLeft(level, j);
        const totalRight = ff.getBarTotalRght(level, j);

        const { leftRatio, rightRatio } = getRatios(viewType, ff, level, j);
        const leftPercent = ratioToPercent(leftRatio);
        const rightPercent = ratioToPercent(rightRatio);

        return {
          format: 'double',
          left: totalLeft,
          right: totalRight,
          title,
          sampleRate: this.state.sampleRate,
          leftPercent,
          rightPercent,
        };
      }

      default:
        throw new Error(`Wrong format ${format}`);
    }
  };

  xyToHighlightData = (x, y) => {
    const ff = this.props.format;
    const { i, j } = this.xyToBar(x, y);

    const level = this.state.levels[i];

    const posX = Math.max(this.tickToX(ff.getBarOffset(level, j)), 0);
    const posY = (i - this.topLevel) * PX_PER_LEVEL;

    const sw = Math.min(
      this.tickToX(ff.getBarOffset(level, j) + ff.getBarTotal(level, j)) - posX,
      this.canvas.clientWidth
    );

    return {
      left: this.canvas.offsetLeft + posX,
      top: this.canvas.offsetTop + posY,
      width: sw,
    };
  };

  isWithinBounds = (x, y) => {
    if (x < 0 || x > this.canvas.clientWidth) {
      return false;
    }

    const { j } = this.xyToBar(x, y);
    if (j === -1) {
      return false;
    }

    return true;
  };

  xyToContextMenuItems = (x, y) => {
    const isFocused = this.selectedLevel !== 0;

    // Depending on what item we clicked
    // The menu items will be completely different
    return [
      <MenuItem key="reset" disabled={!isFocused} onClick={this.reset}>
        Reset View
      </MenuItem>,
    ];
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
                <DiffLegend />
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
              onBlur={() => {}}
            />

            <Highlight
              barHeight={PX_PER_LEVEL}
              canvasRef={this.canvasRef}
              xyToHighlightData={this.xyToHighlightData}
              isWithinBounds={this.isWithinBounds}
            />
          </div>
        </div>

        <ContextMenu
          canvasRef={this.canvasRef}
          items={this.contextMenuItems}
          xyToMenuItems={this.xyToContextMenuItems}
        />

        {this.canvas && (
          <Tooltip
            format={this.props.format.format}
            canvasRef={this.canvasRef}
            xyToData={this.xyToTooltipData}
            isWithinBounds={this.isWithinBounds}
            graphWidth={this.canvas.clientWidth}
            numTicks={this.state.numTicks}
            sampleRate={this.state.sampleRate}
            units={this.state.units}
          />
        )}
      </>
    );
  };
}

export default FlameGraph;
