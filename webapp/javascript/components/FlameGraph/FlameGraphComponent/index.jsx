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
import { PX_PER_LEVEL } from './constants';
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
      viewDiff: props.viewType === 'diff' ? 'diff' : undefined,
    };
    this.canvasRef = React.createRef();
    this.highlightRef = React.createRef();
    this.tooltipRef = React.createRef();
  }

  componentDidMount() {
    window.addEventListener('resize', this.resizeHandler);
    window.addEventListener('focus', this.focusHandler);

    if (this.props.shortcut) {
      this.props.shortcut.registerShortcut(
        this.props.onReset,
        ['escape'],
        'Reset',
        'Reset Flamegraph View'
      );
    }
    this.updateData();

    this.createFlamegraph();
    this.flamegraph.render();
  }

  componentDidUpdate() {
    this.createFlamegraph();
    this.flamegraph.render();
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

  //  reset = () => {
  //    this.props.reset();
  //
  //    //this.flamegraph.reset();
  //    //this.renderCanvas();
  //  };
  //
  onClick = (e) => {
    const { i, j } = this.flamegraph.xyToBar(
      e.nativeEvent.offsetX,
      e.nativeEvent.offsetY
    );
    this.props.onZoom2(i, j);

    //    this.props.onZoom(
    //      this.flamegraph.xyToZoom(e.nativeEvent.offsetX, e.nativeEvent.offsetY)
    //    );
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
  };

  xyToTooltipData = (format, x, y) => {
    return this.flamegraph.xyToBarData(x, y);
  };

  xyToHighlightData = (x, y) => {
    const bar = this.flamegraph.xyToBarPosition(x, y);

    return {
      left: this.canvasRef.current.offsetLeft + bar.x,
      top: this.canvasRef.current.offsetTop + bar.y,
      width: bar.width,
    };
  };

  xyToContextMenuItems = (x, y) => {
    const isDirty = this.props.isDirty();

    //
    //      <MenuItem key="focus" onClick={() => this.focusOnNode(x, y)}>
    //        Focus
    //      </MenuItem>,
    return [
      <MenuItem key="reset" disabled={!isDirty} onClick={this.props.onReset}>
        Reset View
      </MenuItem>,
    ];
  };

  // this is required
  // otherwise may get stale props
  // eg. thinking that a zoomed flamegraph is not zoomed
  isWithinBounds = (x, y) => {
    return this.flamegraph.isWithinBounds(x, y);
  };

  createFlamegraph() {
    this.flamegraph = new Flamegraph(
      this.props.flamebearer,
      this.canvasRef.current,
      this.props.topLevel,
      this.props.rangeMin,
      this.props.rangeMax,
      this.props.selectedLevel,
      this.props.fitMode,
      this.props.query,
      this.props.zoom
    );
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
                isWithinBounds={this.isWithinBounds}
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
            isWithinBounds={this.isWithinBounds}
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
