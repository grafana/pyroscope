/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */

import React from 'react';
import clsx from 'clsx';
import { Option } from 'prelude-ts';
import Graph from './FlameGraphComponent';
import TimelineChartWrapper from '../TimelineChartWrapper';
import ProfilerTable from '../ProfilerTable';
import ProfilerHeader from '../ProfilerHeader';
import { createFF } from '../../util/flamebearer';

import ExportData from '../ExportData';
import { isAbortError } from '../../util/abort';

import InstructionText from './InstructionText';

const paramsToObject = (entries) => {
  const result = {};
  entries.forEach(([key, value]) => {
    result[key] = value;
  });
  return result;
};

const getParamsFromRenderURL = (inputURL) => {
  const urlParamsRegexp = /(.*render\?)(?<urlParams>(.*))/;
  const paramsString = inputURL.match(urlParamsRegexp);

  const params = new URLSearchParams(paramsString.groups.urlParams);
  const paramsObj = paramsToObject([...params.entries()]);
  return paramsObj;
};

class FlameGraphRenderer extends React.Component {
  // TODO: this could come from some other state
  // eg localstorage
  initialFlamegraphState = {
    focusedNode: Option.none(),
    zoom: Option.none(),
  };

  constructor(props) {
    super();
    this.state = {
      isFlamegraphDirty: false,
      sortBy: 'self',
      sortByDirection: 'desc',
      view: 'both',
      viewDiff: props.viewType === 'diff' ? 'diff' : undefined,
      fitMode: props.fitMode ? props.fitMode : 'HEAD',
      flamebearer: null,

      // query used in the 'search' checkbox
      highlightQuery: '',

      flamegraphConfigs: this.initialFlamegraphState,
    };
  }

  componentDidMount() {
    if (this.props.viewSide === 'left' || this.props.viewSide === 'right') {
      this.fetchFlameBearerData(this.props[`${this.props.viewSide}RenderURL`]);
    } else if (this.props.viewType === 'single') {
      this.fetchFlameBearerData(this.props.renderURL);
    } else if (this.props.viewType === 'diff') {
      this.fetchFlameBearerData(this.props.diffRenderURL);
    }
  }

  componentDidUpdate(prevProps, prevState) {
    const propsChanged =
      getParamsFromRenderURL(this.props.renderURL).query !==
        getParamsFromRenderURL(prevProps.renderURL).query ||
      prevProps.maxNodes !== this.props.maxNodes ||
      prevProps.refreshToken !== this.props.refreshToken;

    if (
      propsChanged ||
      prevProps.from !== this.props.from ||
      prevProps.until !== this.props.until ||
      prevProps[`${this.props.viewSide}From`] !==
        this.props[`${this.props.viewSide}From`] ||
      prevProps[`${this.props.viewSide}Until`] !==
        this.props[`${this.props.viewSide}Until`]
    ) {
      if (this.props.viewSide === 'left' || this.props.viewSide === 'right') {
        this.fetchFlameBearerData(
          this.props[`${this.props.viewSide}RenderURL`]
        );
      } else if (this.props.viewType === 'single') {
        this.fetchFlameBearerData(this.props.renderURL);
      }
    }

    if (this.props.viewType === 'diff') {
      if (
        propsChanged ||
        prevProps.leftFrom !== this.props.leftFrom ||
        prevProps.leftUntil !== this.props.leftUntil ||
        prevProps.rightFrom !== this.props.rightFrom ||
        prevProps.rightUntil !== this.props.rightUntil
      ) {
        this.fetchFlameBearerData(this.props.diffRenderURL);
      }
    }

    // flamegraph configs changed
    if (prevState.flamegraphConfigs !== this.state.flamegraphConfigs) {
      this.updateFlamegraphDirtiness();
    }
  }

  componentWillUnmount() {
    this.abortCurrentJSONController();
  }

  updateFitMode = (newFitMode) => {
    this.setState({
      fitMode: newFitMode,
    });
  };

  updateFlamegraphDirtiness = () => {
    const isDirty = this.isDirty();

    this.setState({
      isFlamegraphDirty: isDirty,
    });
  };

  handleSearchChange = (e) => {
    this.setState({
      highlightQuery: e,
    });
  };

  onReset = () => {
    this.setState({
      ...this.state,
      flamegraphConfigs: {
        ...this.state.flamegraphConfigs,
        ...this.initialFlamegraphState,
      },
    });
  };

  updateView = (newView) => {
    this.setState({
      view: newView,
    });
  };

  updateViewDiff = (newView) => {
    this.setState({
      viewDiff: newView,
    });
  };

  onFlamegraphZoom = (bar) => {
    // zooming on the topmost bar is equivalent to resetting to the original state
    if (bar.isSome() && bar.get().i === 0 && bar.get().j === 0) {
      this.onReset();
      return;
    }

    // otherwise just pass it up to the state
    // doesn't matter if it's some or none
    this.setState({
      ...this.state,
      flamegraphConfigs: {
        ...this.state.flamegraphConfigs,
        zoom: bar,
      },
    });
  };

  onFocusOnNode = (i, j) => {
    if (i === 0 && j === 0) {
      this.onReset();
      return;
    }

    let flamegraphConfigs = { ...this.state.flamegraphConfigs };

    // reset zoom if we are focusing below the zoom
    // or the same one we were zoomed
    const { zoom } = this.state.flamegraphConfigs;
    if (zoom.isSome()) {
      if (zoom.get().i <= i) {
        flamegraphConfigs = {
          ...flamegraphConfigs,
          zoom: this.initialFlamegraphState.zoom,
        };
      }
    }

    this.setState({
      ...this.state,
      flamegraphConfigs: {
        ...flamegraphConfigs,
        focusedNode: Option.some({ i, j }),
      },
    });
  };

  updateSortBy = (newSortBy) => {
    let dir = this.state.sortByDirection;
    if (this.state.sortBy === newSortBy) {
      dir = dir === 'asc' ? 'desc' : 'asc';
    } else {
      dir = 'desc';
    }
    this.setState({
      sortBy: newSortBy,
      sortByDirection: dir,
    });
  };

  isDirty = () => {
    // TODO: is this a good idea?
    return (
      JSON.stringify(this.initialFlamegraphState) !==
      JSON.stringify(this.state.flamegraphConfigs)
    );
  };

  parseFormat(format) {
    return createFF(format || this.state.format);
  }

  abortCurrentJSONController() {
    if (this.currentJSONController) {
      this.currentJSONController.abort();
    }
  }

  fetchFlameBearerData(url) {
    // TODO(eh-am):
    // move all this request fetching to a data service layer
    /* eslint-disable no-restricted-syntax*/
    function deltaDiff(levels, start, step) {
      for (const level of levels) {
        let prev = 0;
        for (let i = start; i < level.length; i += step) {
          level[i] += prev;
          prev = level[i] + level[i + 1];
        }
      }
    }
    /* eslint-enable no-restricted-syntax*/

    function deltaDiffWrapper(format, levels) {
      if (format === 'double') {
        deltaDiff(levels, 0, 7);
        deltaDiff(levels, 3, 7);
      } else {
        deltaDiff(levels, 0, 4);
      }
    }
    this.abortCurrentJSONController();
    if (this.currentJSONController) {
      this.currentJSONController.abort();
    }
    this.currentJSONController = new AbortController();

    fetch(`${url}&format=json`, { signal: this.currentJSONController.signal })
      .then((response) => response.json())
      .then((data) => {
        const { flamebearer, leftTicks, rightTicks } = data;

        deltaDiffWrapper(flamebearer.format, flamebearer.levels);

        // conceptually makes sense grouping them at frontend level
        // since these ticks are used to compute stuff (eg colors)
        flamebearer.leftTicks = leftTicks;
        flamebearer.rightTicks = rightTicks;

        // the new flamegraph shouldn't have zoom etc
        this.onReset();
        this.setState({
          flamebearer,
        });
      })
      .catch((e) => {
        // AbortErrors are fine
        if (!isAbortError(e)) {
          throw e;
        }
      })
      .finally();
  }

  render = () => {
    // This is necessary because the order switches depending on single vs comparison view
    const tablePane = (
      <div
        key="table-pane"
        className={clsx('pane', {
          hidden:
            this.state.view === 'icicle' ||
            !this.state.flamebearer ||
            this.state.flamebearer.names.length <= 1,
          'vertical-orientation': this.props.viewType === 'double',
        })}
      >
        <ProfilerTable
          data-testid="table-view"
          flamebearer={this.state.flamebearer}
          sortByDirection={this.state.sortByDirection}
          sortBy={this.state.sortBy}
          updateSortBy={this.updateSortBy}
          view={this.state.view}
          viewDiff={this.state.viewDiff}
          fitMode={this.state.fitMode}
          isFlamegraphDirty={this.state.isFlamegraphDirty}
        />
      </div>
    );
    const dataExists =
      this.state.view !== 'table' ||
      (this.state.flamebearer && this.state.flamebearer.names.length <= 1);

    const flameGraphPane =
      this.state.flamebearer && dataExists ? (
        <Graph
          key="flamegraph-pane"
          flamebearer={this.state.flamebearer}
          format={this.parseFormat(this.state.flamebearer.format)}
          view={this.state.view}
          ExportData={ExportData}
          highlightQuery={this.state.highlightQuery}
          fitMode={this.state.fitMode}
          viewType={this.props.viewType}
          zoom={this.state.flamegraphConfigs.zoom}
          focusedNode={this.state.flamegraphConfigs.focusedNode}
          label={this.props.query}
          onZoom={this.onFlamegraphZoom}
          onFocusOnNode={this.onFocusOnNode}
          onReset={this.onReset}
          isDirty={this.isDirty}
        />
      ) : null;

    const panes =
      this.props.viewType === 'double'
        ? [flameGraphPane, tablePane]
        : [tablePane, flameGraphPane];

    // const flotData = this.props.timeline
    //   ? [this.props.timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
    //   : [];

    return (
      <div
        className={clsx('canvas-renderer', {
          double: this.props.viewType === 'double',
        })}
      >
        <div className="canvas-container">
          <ProfilerHeader
            view={this.state.view}
            viewDiff={this.state.viewDiff}
            handleSearchChange={this.handleSearchChange}
            reset={this.onReset}
            updateView={this.updateView}
            updateViewDiff={this.updateViewDiff}
            updateFitMode={this.updateFitMode}
            fitMode={this.state.fitMode}
            isFlamegraphDirty={this.state.isFlamegraphDirty}
            selectedNode={this.state.flamegraphConfigs.zoom}
            onFocusOnSubtree={(i, j) => {
              this.onFocusOnNode(i, j);
            }}
          />
          {this.props.viewType === 'double' ? (
            <>
              <InstructionText {...this.props} />
              <TimelineChartWrapper
                key={`timeline-chart-${this.props.viewSide}`}
                id={`timeline-chart-${this.props.viewSide}`}
                viewSide={this.props.viewSide}
              />
            </>
          ) : this.props.viewType === 'diff' ? (
            <>
              <div className="diff-instructions-wrapper">
                <div className="diff-instructions-wrapper-side">
                  <InstructionText {...this.props} viewSide="left" />
                  <TimelineChartWrapper
                    key="timeline-chart-left"
                    id="timeline-chart-left"
                    viewSide="left"
                  />
                </div>
                <div className="diff-instructions-wrapper-side">
                  <InstructionText {...this.props} viewSide="right" />
                  <TimelineChartWrapper
                    key="timeline-chart-right"
                    id="timeline-chart-right"
                    viewSide="right"
                  />
                </div>
              </div>
            </>
          ) : null}
          <div
            className={clsx('flamegraph-container panes-wrapper', {
              'vertical-orientation': this.props.viewType === 'double',
            })}
          >
            {panes.map((pane) => pane)}
            {/* { tablePane }
            { flameGraphPane } */}
          </div>
        </div>
      </div>
    );
  };
}

export default FlameGraphRenderer;
