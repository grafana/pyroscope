/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */

import React, { Dispatch, SetStateAction } from 'react';
import clsx from 'clsx';
import { Maybe } from 'true-myth';
import { createFF, Flamebearer, Profile } from '@pyroscope/models';
import Graph from './FlameGraphComponent';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: let's move this to typescript some time in the future
import ProfilerTable from '../ProfilerTable';
import Toolbar from '../Toolbar';
import NoProfilingData from '../NoProfilingData';
import { DefaultPalette } from './FlameGraphComponent/colorPalette';
import styles from './FlamegraphRenderer.module.scss';
import PyroscopeLogo from '../logo-v3-small.svg';
import decode from './decode';
import { FitModes } from '../fitMode/fitMode';
import { ViewTypes } from './FlameGraphComponent/viewTypes';

// Still support old flamebearer format
// But prefer the new 'profile' one
function mountFlamebearer(p: { profile?: Profile; flamebearer?: Flamebearer }) {
  if (p.profile && p.flamebearer) {
    console.warn(
      "'profile' and 'flamebearer' properties are mutually exclusible. Preferring profile."
    );
  }

  if (p.profile) {
    const copy = JSON.parse(JSON.stringify(p.profile));
    const profile = decode(copy);

    return {
      ...profile,
      ...profile.flamebearer,
      ...profile.metadata,
    } as Flamebearer;
  }

  if (p.flamebearer) {
    return p.flamebearer;
  }

  // people may send us both values as undefined
  // but we still have to render something
  const noop: Flamebearer = {
    format: 'single',
    names: [],
    units: '',
    levels: [[]],
    spyName: '',
    numTicks: 0,
    sampleRate: 0,
  };
  return noop;
}

// Refers to a node in the flamegraph
interface Node {
  i: number;
  j: number;
}

export interface FlamegraphRendererProps {
  /** in case you ONLY want to display a specific visualization mode. It will also disable the dropdown that allows you to change mode. */
  profile?: Profile;
  onlyDisplay?: ViewTypes;
  showToolbar?: boolean;

  /** whether to display the panes (table and flamegraph) side by side ('horizontal') or one on top of the other ('vertical') */
  panesOrientation?: 'horizontal' | 'vertical';
  showPyroscopeLogo?: boolean;
  renderLogo?: boolean;
  ExportData?: React.ComponentProps<typeof Graph>['ExportData'];
  colorMode?: 'light' | 'dark';

  /** @deprecated  prefer Profile */
  flamebearer?: Flamebearer;
  sharedQuery?: {
    searchQuery?: string;
    onQueryChange: Dispatch<SetStateAction<string | undefined>>;
    syncEnabled: string | boolean;
    toggleSync: Dispatch<SetStateAction<boolean | string>>;
    id: string;
  };
}

interface FlamegraphRendererState {
  isFlamegraphDirty: boolean;
  sortBy: 'self' | 'total' | 'selfDiff' | 'totalDiff';
  sortByDirection: 'desc' | 'asc';

  view: NonNullable<FlamegraphRendererProps['onlyDisplay']>;
  panesOrientation: NonNullable<FlamegraphRendererProps['panesOrientation']>;

  viewDiff: 'diff' | 'total' | 'self';
  fitMode: 'HEAD' | 'TAIL';
  flamebearer: NonNullable<FlamegraphRendererProps['flamebearer']>;
  highlightQuery: string;

  flamegraphConfigs: {
    focusedNode: Maybe<Node>;
    zoom: Maybe<Node>;
  };

  palette: typeof DefaultPalette;
}

class FlameGraphRenderer extends React.Component<
  FlamegraphRendererProps,
  FlamegraphRendererState
> {
  resetFlamegraphState = {
    focusedNode: Maybe.nothing<Node>(),
    zoom: Maybe.nothing<Node>(),
  };

  // TODO: At some point the initial state may be set via the user
  // Eg when sharing a specific node
  initialFlamegraphState = this.resetFlamegraphState;

  constructor(props: FlamegraphRendererProps) {
    super(props);

    this.state = {
      isFlamegraphDirty: false,
      sortBy: 'self',
      sortByDirection: 'desc',
      view: this.props.onlyDisplay ? this.props.onlyDisplay : 'both',
      viewDiff: 'diff',
      fitMode: 'HEAD',
      flamebearer: mountFlamebearer(props),

      // Default to horizontal since it's the most common case
      panesOrientation: props.panesOrientation
        ? props.panesOrientation
        : 'horizontal',

      // query used in the 'search' checkbox
      highlightQuery: '',

      flamegraphConfigs: this.initialFlamegraphState,

      // TODO make this come from the redux store?
      palette: DefaultPalette,
    };
  }

  componentDidUpdate(
    prevProps: FlamegraphRendererProps,
    prevState: FlamegraphRendererState
  ) {
    // TODO: this is a slow operation
    const prevFlame = mountFlamebearer(prevProps);
    const currFlame = mountFlamebearer(this.props);

    if (!this.isSameFlamebearer(prevFlame, currFlame)) {
      const newConfigs = this.calcNewConfigs(prevFlame, currFlame);

      // Batch these updates to not do unnecessary work
      // eslint-disable-next-line react/no-did-update-set-state
      this.setState({
        flamebearer: currFlame,

        flamegraphConfigs: {
          ...this.state.flamegraphConfigs,
          ...newConfigs,
        },
      });
      return;
    }

    // flamegraph configs changed
    if (prevState.flamegraphConfigs !== this.state.flamegraphConfigs) {
      this.updateFlamegraphDirtiness();
    }
  }

  // Calculate what should be the new configs
  // It checks if the zoom/selectNode still points to the same node
  // If not, it resets to the resetFlamegraphState
  calcNewConfigs = (prevFlame: Flamebearer, currFlame: Flamebearer) => {
    const newConfigs = this.state.flamegraphConfigs;

    // This is a simple heuristic based on the name
    // It does not account for eg recursive calls
    const isSameNode = (f: Flamebearer, f2: Flamebearer, s: Maybe<Node>) => {
      // TODO: don't use createFF directly
      const getBarName = (f: Flamebearer, i: number, j: number) => {
        return f.names[createFF(f.format).getBarName(f.levels[i], j)];
      };

      // No node is technically the same node
      if (s.isNothing) {
        return true;
      }

      // if the bar doesn't exist, it will throw an error
      try {
        const barName1 = getBarName(f, s.value.i, s.value.j);
        const barName2 = getBarName(f2, s.value.i, s.value.j);
        return barName1 === barName2;
      } catch {
        return false;
      }
    };

    // Reset zoom
    const currZoom = this.state.flamegraphConfigs.zoom;
    if (!isSameNode(prevFlame, currFlame, currZoom)) {
      newConfigs.zoom = this.resetFlamegraphState.zoom;
    }

    // Reset focused node
    const currFocusedNode = this.state.flamegraphConfigs.focusedNode;
    if (!isSameNode(prevFlame, currFlame, currFocusedNode)) {
      newConfigs.focusedNode = this.resetFlamegraphState.focusedNode;
    }

    return newConfigs;
  };

  handleSearchChange = (e: string) => {
    this.setState({
      highlightQuery: e,
    });
  };

  isSameFlamebearer = (prevFlame: Flamebearer, currFlame: Flamebearer) => {
    // This is a poor heuristic, but it should work most of the times
    return prevFlame.numTicks === currFlame.numTicks;
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

  onFlamegraphZoom = (bar: Maybe<Node>) => {
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

  onFocusOnNode = (i: number, j: number) => {
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
  onTableItemClick = (tableItem: { name: string }) => {
    let { name } = tableItem;

    if (tableItem.name === this.state.highlightQuery) {
      name = '';
    }
    this.handleSearchChange(name);
  };

  updateSortBy = (newSortBy: FlamegraphRendererState['sortBy']) => {
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

  // This in fact seems refers to the diff table
  updateViewDiff = (newView: 'total' | 'self' | 'diff') => {
    this.setState({
      viewDiff: newView,
    });
  };

  updateView = (newView: ViewTypes) => {
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

  updateFitMode = (newFitMode: FitModes) => {
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

  render = () => {
    // This is necessary because the order switches depending on single vs comparison view
    const tablePane = (
      <div
        key="table-pane"
        className={clsx(
          styles.tablePane,
          this.state.panesOrientation === 'vertical'
            ? styles.vertical
            : styles.horizontal
        )}
      >
        <ProfilerTable
          data-testid="table-view"
          flamebearer={this.state.flamebearer}
          sortByDirection={this.state.sortByDirection}
          sortBy={this.state.sortBy}
          updateSortBy={this.updateSortBy}
          view={this.state.view}
          viewDiff={
            this.state.flamebearer?.format === 'double' && this.state.viewDiff
          }
          fitMode={this.state.fitMode}
          isFlamegraphDirty={this.state.isFlamegraphDirty}
          highlightQuery={this.state.highlightQuery}
          handleTableItemClick={this.onTableItemClick}
          palette={this.state.palette}
        />
      </div>
    );

    //    const flamegraphDataTestId = figureFlamegraphDataTestId(
    //      this.props.viewType,
    //      this.props.viewSide
    //    );

    const toolbarVisible = this.shouldShowToolbar();

    const flameGraphPane = (
      <Graph
        key="flamegraph-pane"
        // data-testid={flamegraphDataTestId}
        flamebearer={this.state.flamebearer}
        ExportData={this.props.ExportData || <></>}
        highlightQuery={this.state.highlightQuery}
        fitMode={this.state.fitMode}
        zoom={this.state.flamegraphConfigs.zoom}
        focusedNode={this.state.flamegraphConfigs.focusedNode}
        onZoom={this.onFlamegraphZoom}
        onFocusOnNode={this.onFocusOnNode}
        onReset={this.onReset}
        isDirty={this.isDirty}
        palette={this.state.palette}
        toolbarVisible={toolbarVisible}
        setPalette={(p) =>
          this.setState({
            palette: p,
          })
        }
      />
    );

    const dataUnavailable =
      !this.state.flamebearer || this.state.flamebearer.names.length <= 1;
    const panes = decidePanesOrder(this.state.view, flameGraphPane, tablePane);

    return (
      <div
        className="flamegraph-root"
        data-flamegraph-color-mode={this.props.colorMode || 'dark'}
      >
        <div>
          {toolbarVisible && (
            <Toolbar
              sharedQuery={this.props.sharedQuery}
              renderLogo={this.props.renderLogo || false}
              disableChangingDisplay={!!this.props.onlyDisplay}
              flamegraphType={this.state.flamebearer.format}
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
              highlightQuery={this.state.highlightQuery}
              onFocusOnSubtree={this.onFocusOnNode}
            />
          )}
          {this.props.children}
          <div
            className={`${styles.flamegraphContainer} ${clsx(
              this.state.panesOrientation === 'vertical'
                ? styles.vertical
                : styles.horizontal,
              styles[this.state.panesOrientation],
              styles.panesWrapper
            )}`}
          >
            {dataUnavailable ? <NoProfilingData /> : panes.map((pane) => pane)}
          </div>
        </div>

        {this.props.showPyroscopeLogo && (
          <div className={styles.createdBy}>
            Created by
            <a
              href="https://twitter.com/PyroscopeIO"
              rel="noreferrer"
              target="_blank"
            >
              <PyroscopeLogo width="30" height="30" />
              @PyroscopeIO
            </a>
          </div>
        )}
      </div>
    );
  };
}

function decidePanesOrder(
  view: FlamegraphRendererState['view'],
  flamegraphPane: JSX.Element | null,
  tablePane: JSX.Element
) {
  switch (view) {
    case 'table': {
      return [tablePane];
    }
    case 'flamegraph': {
      return [flamegraphPane];
    }

    case 'both': {
      return [tablePane, flamegraphPane];
    }
    default: {
      throw new Error(`Invalid view '${view}'`);
    }
  }
}

export default FlameGraphRenderer;
