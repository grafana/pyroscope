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
import DiffLegend from './DiffLegend';
import Tooltip from './Tooltip';
import Highlight from './Highlight';
import ContextMenu from './ContextMenu';
import { PX_PER_LEVEL, COLLAPSE_THRESHOLD, BAR_HEIGHT } from './constants';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';

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
    this.highlightRef = React.createRef();
    this.tooltipRef = React.createRef();
    this.currentJSONController = null;
  }

  componentDidMount() {
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

    this.flamegraph = new Flamegraph(
      this.props.flamebearer,
      this.canvasRef.current,
      'HEAD'
    );
    this.flamegraph.render();
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
    this.flamegraph.reset();
    this.renderCanvas();
  };

  onClick = (e) => {
    const { i, j } = this.flamegraph.xyToBar(
      e.nativeEvent.offsetX,
      e.nativeEvent.offsetY
    );
    if (j === -1) return;

    this.flamegraph.zoom(i, j);
    this.flamegraph.render();
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

  renderCanvas = () => {
    this.flamegraph.render();

    this.graphWidth = this.canvasRef.current.width;
    this.pxPerTick =
      this.graphWidth /
      this.props.flamebearer.numTicks /
      (this.rangeMax - this.rangeMin);
  };

  xyToTooltipData = (format, x, y) => {
    return this.flamegraph.xyToBarData(x, y);
  };

  xyToHighlightData = (x, y) => {
    const bar = this.flamegraph.xyToBarPosition(x, y);

    return {
      left: this.flamegraph.getCanvas().offsetLeft + bar.x,
      top: this.flamegraph.getCanvas().offsetTop + bar.y,
      width: bar.width,
    };
  };

  xyToContextMenuItems = (x, y) => {
    const isSelected = this.selectedLevel !== 0 || this.topLevel !== 0;

    return [
      <MenuItem key="reset" disabled={!isSelected} onClick={this.reset}>
        Reset View
      </MenuItem>,
      <MenuItem key="focus" onClick={() => this.focusOnNode(x, y)}>
        Focus
      </MenuItem>,
    ];
  };

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
              height="0"
              data-testid="flamegraph-canvas"
              data-appname={this.props.label}
              className={`flamegraph-canvas ${styles.hover}`}
              ref={this.canvasRef}
              onClick={this.onClick}
              onBlur={() => {}}
            />

            {this.flamegraph && (
              <Highlight
                barHeight={PX_PER_LEVEL}
                canvasRef={this.canvasRef}
                xyToHighlightData={this.xyToHighlightData}
                isWithinBounds={this.flamegraph.isWithinBounds}
              />
            )}
          </div>
        </div>

        <ContextMenu
          canvasRef={this.canvasRef}
          items={this.contextMenuItems}
          xyToMenuItems={this.xyToContextMenuItems}
        />

        {this.canvasRef.current && (
          <Tooltip
            format={this.props.format.format}
            canvasRef={this.canvasRef}
            xyToData={this.xyToTooltipData}
            isWithinBounds={this.flamegraph.isWithinBounds}
            graphWidth={this.graphWidth}
            numTicks={this.props.flamebearer.numTicks}
            sampleRate={this.props.flamebearer.sampleRate}
            leftTicks={this.props.flamebearer.leftTicks}
            rightTicks={this.props.flamebearer.rightTicks}
            units={this.state.units}
          />
        )}
      </>
    );
  };
}

export default FlameGraph;
