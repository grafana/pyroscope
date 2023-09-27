/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */
/* eslint-disable global-require */

import React, { Dispatch, SetStateAction, ReactNode, Component } from 'react';
import clsx from 'clsx';
import { Maybe } from 'true-myth';
import { createFF, Flamebearer, Profile } from '@pyroscope/legacy/models';
import NoData from '@pyroscope/ui/NoData';

import Graph from './FlameGraphComponent';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: let's move this to typescript some time in the future
import ProfilerTable from '../ProfilerTable';
import Toolbar, { ProfileHeaderProps } from '../Toolbar';
import {
  calleesProfile,
  callersProfile,
} from '../convert/sandwichViewProfiles';
import { DefaultPalette } from './FlameGraphComponent/colorPalette';
import styles from './FlamegraphRenderer.module.scss';
import PyroscopeLogo from '../logo-v3-small.svg';
import { FitModes } from '../fitMode/fitMode';
import { ViewTypes } from './FlameGraphComponent/viewTypes';
import { isSameFlamebearer } from './uniqueness';
import { normalize } from './normalize';

// Refers to a node in the flamegraph
interface Node {
  i: number;
  j: number;
}

export type SharedQuery = {
  searchQuery?: string;
  onQueryChange: Dispatch<SetStateAction<string | undefined>>;
  syncEnabled: string | boolean;
  toggleSync: Dispatch<SetStateAction<boolean | string>>;
  id: string;
};

export interface FlamegraphRendererProps {
  profile?: Profile;

  /** in case you ONLY want to display a specific visualization mode. It will also disable the dropdown that allows you to change mode. */
  onlyDisplay?: ViewTypes;
  showToolbar?: boolean;

  /** whether to display the panes (table and flamegraph) side by side ('horizontal') or one on top of the other ('vertical') */
  panesOrientation?: 'horizontal' | 'vertical';
  showPyroscopeLogo?: boolean;
  showCredit?: boolean;
  ExportData?: ProfileHeaderProps['ExportData'];

  /** @deprecated  prefer Profile */
  flamebearer?: Flamebearer;
  sharedQuery?: SharedQuery;

  children?: ReactNode;
}

interface FlamegraphRendererState {
  /** A dirty flamegraph refers to a flamegraph where its original state can be reset */
  isFlamegraphDirty: boolean;

  view: NonNullable<FlamegraphRendererProps['onlyDisplay']>;
  panesOrientation: NonNullable<FlamegraphRendererProps['panesOrientation']>;

  fitMode: 'HEAD' | 'TAIL';
  flamebearer: NonNullable<FlamegraphRendererProps['flamebearer']>;

  /** Query searched in the input box.
   * It's used to filter data in the table AND highlight items in the flamegraph */
  searchQuery: string;
  /** Triggered when an item is clicked on the table. It overwrites the searchQuery */
  selectedItem: Maybe<string>;

  flamegraphConfigs: {
    focusedNode: Maybe<Node>;
    zoom: Maybe<Node>;
  };

  palette: typeof DefaultPalette;
}

class FlameGraphRenderer extends Component<
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

  // eslint-disable-next-line react/static-property-placement
  static defaultProps = {
    showCredit: true,
  };

  constructor(props: FlamegraphRendererProps) {
    super(props);

    this.state = {
      isFlamegraphDirty: false,
      view: this.props.onlyDisplay ? this.props.onlyDisplay : 'both',
      fitMode: 'HEAD',
      flamebearer: normalize(props),

      // Default to horizontal since it's the most common case
      panesOrientation: props.panesOrientation
        ? props.panesOrientation
        : 'horizontal',

      // query used in the 'search' checkbox
      searchQuery: '',
      selectedItem: Maybe.nothing(),

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
    const prevFlame = normalize(prevProps);
    const currFlame = normalize(this.props);

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
        selectedItem: Maybe.nothing(),
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

  onSearchChange = (e: string) => {
    this.setState({ searchQuery: e });
  };

  isSameFlamebearer = (prevFlame: Flamebearer, currFlame: Flamebearer) => {
    return isSameFlamebearer(prevFlame, currFlame);
    // TODO: come up with a less resource intensive operation
    // keep in mind naive heuristics may provide bad behaviours like (https://github.com/pyroscope-io/pyroscope/issues/1192)
    //    return JSON.stringify(prevFlame) === JSON.stringify(currFlame);
  };

  onReset = () => {
    this.setState({
      ...this.state,
      flamegraphConfigs: {
        ...this.state.flamegraphConfigs,
        ...this.initialFlamegraphState,
      },
      selectedItem: Maybe.nothing(),
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

  setActiveItem = (item: { name: string }) => {
    const { name } = item;

    // if clicking on the same item, undo the search
    if (this.state.selectedItem.isJust) {
      if (name === this.state.selectedItem.value) {
        this.setState({
          selectedItem: Maybe.nothing(),
        });
        return;
      }
    }

    // clicking for the first time
    this.setState({
      selectedItem: Maybe.just(name),
    });
  };

  getHighlightQuery = () => {
    // prefer table selected
    if (this.state.selectedItem.isJust) {
      return this.state.selectedItem.value;
    }

    return this.state.searchQuery;
  };

  updateView = (newView: ViewTypes) => {
    if (newView === 'sandwich') {
      this.setState({
        searchQuery: '',
        flamegraphConfigs: this.resetFlamegraphState,
      });
    }

    this.setState({
      view: newView,
    });
  };

  updateFlamegraphDirtiness = () => {
    // TODO(eh-am): find a better approach
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

  // used as a variable instead of keeping in the state
  // so that the flamegraph doesn't rerender unnecessarily
  isDirty = () => {
    return (
      this.state.selectedItem.isJust ||
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
          fitMode={this.state.fitMode}
          highlightQuery={this.state.searchQuery}
          selectedItem={this.state.selectedItem}
          handleTableItemClick={this.setActiveItem}
          palette={this.state.palette}
        />
      </div>
    );

    const toolbarVisible = this.shouldShowToolbar();

    const flameGraphPane = (
      <Graph
        key="flamegraph-pane"
        // data-testid={flamegraphDataTestId}
        showCredit={this.props.showCredit as boolean}
        flamebearer={this.state.flamebearer}
        highlightQuery={this.getHighlightQuery()}
        setActiveItem={this.setActiveItem}
        selectedItem={this.state.selectedItem}
        updateView={this.props.onlyDisplay ? undefined : this.updateView}
        fitMode={this.state.fitMode}
        updateFitMode={this.updateFitMode}
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

    const sandwichPane = (() => {
      if (this.state.selectedItem.isNothing) {
        return (
          <div className={styles.sandwichPane} key="sandwich-pane">
            <div
              className={clsx(
                styles.sandwichPaneInfo,
                this.state.panesOrientation === 'vertical'
                  ? styles.vertical
                  : styles.horizontal
              )}
            >
              <div className={styles.arrow} />
              Select a function to view callers/callees sandwich view
            </div>
          </div>
        );
      }

      const callersFlamebearer = callersProfile(
        this.state.flamebearer,
        this.state.selectedItem.value
      );
      const calleesFlamebearer = calleesProfile(
        this.state.flamebearer,
        this.state.selectedItem.value
      );
      const sandwitchGraph = (myCustomParams: {
        flamebearer: Flamebearer;
        headerVisible?: boolean;
        showSingleLevel?: boolean;
      }) => (
        <Graph
          disableClick
          showCredit={this.props.showCredit as boolean}
          highlightQuery=""
          setActiveItem={this.setActiveItem}
          selectedItem={this.state.selectedItem}
          fitMode={this.state.fitMode}
          updateFitMode={this.updateFitMode}
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
          {...myCustomParams}
        />
      );

      return (
        <div className={styles.sandwichPane} key="sandwich-pane">
          <div className={styles.sandwichTop}>
            <span className={styles.name}>Callers</span>
            {/* todo(dogfrogfog): to allow left/right click on the node we should
            store Graph component we clicking and append action only on to
            this component
            will be implemented i nnext PR */}
            {sandwitchGraph({ flamebearer: callersFlamebearer })}
          </div>
          <div className={styles.sandwichBottom}>
            <span className={styles.name}>Callees</span>
            {sandwitchGraph({
              flamebearer: calleesFlamebearer,
              headerVisible: false,
              showSingleLevel: true,
            })}
          </div>
        </div>
      );
    })();

    // export type Flamebearer = {
    //   /**
    //    * List of names
    //    */
    //   names: string[];
    //   /**
    //    * List of level
    //    *
    //    * This is NOT the same as in the flamebearer
    //    * that we receive from the server.
    //    * As in there are some transformations required
    //    * (see deltaDiffWrapper)
    //    */
    //   levels: number[][];
    //   numTicks: number;
    //   maxSelf: number;

    //   /**
    //    * Sample Rate, used in text information
    //    */
    //   sampleRate: number;
    //   units: Units;

    //   spyName: SpyName;
    //   // format: 'double' | 'single';
    //   //  leftTicks?: number;
    //   //  rightTicks?: number;
    // } & addTicks;

    const dataUnavailable =
      !this.state.flamebearer || this.state.flamebearer.names.length <= 1;
    const panes = decidePanesOrder(
      this.state.view,
      flameGraphPane,
      tablePane,
      sandwichPane
    );

    return (
      <div>
        <div>
          {toolbarVisible && (
            <Toolbar
              sharedQuery={this.props.sharedQuery}
              enableChangingDisplay={!this.props.onlyDisplay}
              flamegraphType={this.state.flamebearer.format}
              view={this.state.view}
              handleSearchChange={this.onSearchChange}
              reset={this.onReset}
              updateView={this.updateView}
              updateFitMode={this.updateFitMode}
              fitMode={this.state.fitMode}
              isFlamegraphDirty={this.isDirty()}
              selectedNode={this.state.flamegraphConfigs.zoom}
              highlightQuery={this.state.searchQuery}
              onFocusOnSubtree={this.onFocusOnNode}
              ExportData={this.props.ExportData}
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
            {dataUnavailable ? <NoData /> : panes.map((pane) => pane)}
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
  tablePane: JSX.Element,
  sandwichPane: JSX.Element
) {
  switch (view) {
    case 'table': {
      return [tablePane];
    }
    case 'flamegraph': {
      return [flamegraphPane];
    }
    case 'sandwich': {
      return [tablePane, sandwichPane];
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
