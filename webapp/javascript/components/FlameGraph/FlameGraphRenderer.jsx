/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */

import React from 'react';
import clsx from 'clsx';
import { Maybe } from '@utils/fp';
import Graph from './FlameGraphComponent';
import ProfilerTable from '../ProfilerTable';
import Toolbar from '../Toolbar';
import { createFF } from '../../util/flamebearer';
import {
  DefaultPalette,
  ColorBlindPalette,
} from './FlameGraphComponent/colorPalette';
import styles from './FlamegraphRenderer.module.css';

import ExportData from '../ExportData';

class FlameGraphRenderer extends React.Component {
  // TODO: this could come from some other state
  // eg localstorage
  initialFlamegraphState = {
    focusedNode: Maybe.nothing(),
    zoom: Maybe.nothing(),
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
      flamebearer: props.flamebearer,

      // query used in the 'search' checkbox
      highlightQuery: '',

      flamegraphConfigs: this.initialFlamegraphState,

      // TODO make this come from the redux store?
      palette: DefaultPalette,
    };

    // for situations like in grafana we only display the flamegraph
    // 'both' | 'flamegraph' | 'table'
    this.display = props.display !== undefined ? props.display : 'both';
  }

  componentDidUpdate(prevProps, prevState) {
    const previousFlamebearer = prevProps.flamebearer;
    const actualFlamebearer = this.props.flamebearer;
    if (previousFlamebearer !== actualFlamebearer) {
      this.updateFlamebearerData();
    }

    // flamegraph configs changed
    if (prevState.flamegraphConfigs !== this.state.flamegraphConfigs) {
      this.updateFlamegraphDirtiness();
    }
  }

  componentWillUnmount() {
    this.abortCurrentJSONController();
  }

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

  onFlamegraphZoom = (bar) => {
    // zooming on the topmost bar is equivalent to resetting to the original state
    if (bar.isJust && bar.value.i === 0 && bar.value.j === 0) {
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
    if (zoom.isJust) {
      if (zoom.value.i <= i) {
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
        focusedNode: Maybe.just({ i, j }),
      },
    });
  };

  // if clicking on the same item, undo the search
  onTableItemClick = (tableItem) => {
    let { name } = tableItem;

    if (tableItem.name === this.state.highlightQuery) {
      name = '';
    }
    this.handleSearchChange(name);
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

  updateViewDiff = (newView) => {
    this.setState({
      viewDiff: newView,
    });
  };

  updateView = (newView) => {
    this.setState({
      view: newView,
    });
  };

  updateFlamegraphDirtiness = () => {
    const isDirty = this.isDirty();

    this.setState({
      isFlamegraphDirty: isDirty,
    });
  };

  updateFitMode = (newFitMode) => {
    this.setState({
      fitMode: newFitMode,
    });
  };

  isDirty = () => {
    // TODO: is this a good idea?
    return (
      JSON.stringify(this.initialFlamegraphState) !==
      JSON.stringify(this.state.flamegraphConfigs)
    );
  };

  shouldShowToolbar() {
    // default to true
    return this.props.showToolbar !== undefined ? this.props.showToolbar : true;
  }

  updateFlamebearerData() {
    this.setState({
      flamebearer: this.props.flamebearer,
    });
  }

  parseFormat(format) {
    return createFF(format || this.state.format);
  }

  abortCurrentJSONController() {
    if (this.currentJSONController) {
      this.currentJSONController.abort();
    }
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
          highlightQuery={this.state.highlightQuery}
          handleTableItemClick={this.onTableItemClick}
          palette={this.state.palette}
        />
      </div>
    );
    const dataExists =
      this.state.view !== 'table' ||
      (this.state.flamebearer && this.state.flamebearer.names.length <= 1);

    const flamegraphDataTestId = figureFlamegraphDataTestId(
      this.props.viewType,
      this.props.viewSide
    );

    const exportData = () => {
      // the main idea is to disable exporing that for grafana plugins
      if (this.props.disableExportData) {
        return null;
      }

      if (!this.state.flamebearer) {
        return <ExportData />;
      }

      if (!this.props.rawFlamegraph) {
        return <ExportData />;
      }

      // we only want to download single ones
      if (this.state.flamebearer.format === 'double') {
        return <ExportData />;
      }

      return <ExportData exportFlamebearer={this.props.rawFlamegraph} />;
    };

    const flameGraphPane =
      this.state.flamebearer && dataExists ? (
        <Graph
          key="flamegraph-pane"
          data-testid={flamegraphDataTestId}
          flamebearer={this.state.flamebearer}
          format={this.parseFormat(this.state.flamebearer.format)}
          view={this.state.view}
          ExportData={exportData}
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
          palette={this.state.palette}
          setPalette={(p) =>
            this.setState({
              palette: p,
            })
          }
        />
      ) : null;

    const panes = decidePanesOrder(
      this.props.viewType,
      this.display,
      flameGraphPane,
      tablePane
    );

    return (
      <div
        className={clsx('canvas-renderer', {
          double: this.props.viewType === 'double',
        })}
      >
        <div className="canvas-container">
          {this.shouldShowToolbar() && (
            <Toolbar
              view={this.state.view}
              viewDiff={this.state.viewDiff}
              display={this.props.display}
              handleSearchChange={this.handleSearchChange}
              reset={this.onReset}
              updateView={this.updateView}
              updateViewDiff={this.updateViewDiff}
              updateFitMode={this.updateFitMode}
              fitMode={this.state.fitMode}
              isFlamegraphDirty={this.state.isFlamegraphDirty}
              selectedNode={this.state.flamegraphConfigs.zoom}
              highlightQuery={this.state.highlightQuery}
              onFocusOnSubtree={(i, j) => {
                this.onFocusOnNode(i, j);
              }}
            />
          )}
          {this.props.children}
          <div
            className={`${styles.flamegraphContainer} ${clsx(
              'flamegraph-container panes-wrapper',
              {
                'vertical-orientation': this.props.viewType === 'double',
              }
            )}`}
          >
            {panes.map((pane) => pane)}
          </div>
        </div>
      </div>
    );
  };
}

function decidePanesOrder(viewType, display, flamegraphPane, tablePane) {
  switch (display) {
    case 'table': {
      return [tablePane];
    }
    case 'flamegraph': {
      return [flamegraphPane];
    }

    case 'both':
    default: {
      switch (viewType) {
        case 'double':
          return [flamegraphPane, tablePane];
        default:
          return [tablePane, flamegraphPane];
      }
    }
  }
}

function figureFlamegraphDataTestId(viewType, viewSide) {
  switch (viewType) {
    case 'single': {
      return `flamegraph-single`;
    }
    case 'double': {
      return `flamegraph-comparison-${viewSide}`;
    }
    case 'diff': {
      return `flamegraph-diff`;
    }

    default:
      throw new Error(`Unsupported viewType: ${viewType}`);
  }
}

export default FlameGraphRenderer;
