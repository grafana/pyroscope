/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */

import React from "react";
import clsx from "clsx";
import TimelineChartWrapper from "../TimelineChartWrapper";
import ProfilerTable from "../ProfilerTable";
import ProfilerHeader from "../ProfilerHeader";
import {
  deltaDiffWrapper,
  parseFlamebearerFormat,
} from "../../util/flamebearer";

import InstructionText from "./InstructionText";
import Graph from "./FlameGraph";

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
  const paramsObj = paramsToObject(params);

  return paramsObj;
};

class FlameGraphRenderer extends React.Component {
  constructor(props) {
    super();
    this.state = {
      resetStyle: { visibility: "hidden" },
      sortBy: "self",
      sortByDirection: "desc",
      view: "both",
      viewDiff: props.viewType === "diff" ? "diff" : undefined,
      flamebearer: null,
    };
  }

  componentDidMount() {
    if (this.props.viewSide === "left" || this.props.viewSide === "right") {
      this.fetchFlameBearerData(this.props[`${this.props.viewSide}RenderURL`]);
    } else if (this.props.viewType === "single") {
      this.fetchFlameBearerData(this.props.renderURL);
    } else if (this.props.viewType === "diff") {
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
      if (this.props.viewSide === "left" || this.props.viewSide === "right") {
        this.fetchFlameBearerData(
          this.props[`${this.props.viewSide}RenderURL`]
        );
      } else if (this.props.viewType === "single") {
        this.fetchFlameBearerData(this.props.renderURL);
      }
    }

    if (this.props.viewType === "diff") {
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
  };

  reset = () => {
    this.updateZoom(0, 0);
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

  updateSortBy(newSortBy) {
    let dir = this.state.sortByDirection;
    if (this.state.sortBy === newSortBy) {
      dir = dir === "asc" ? "desc" : "asc";
    } else {
      dir = "desc";
    }
    this.setState({
      sortBy: newSortBy,
      sortByDirection: dir,
    });
  }

  updateZoom(i, j) {
    const ff = this.parseFormat();
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
    this.updateResetStyle();
  }

  parseFormat(format) {
    return parseFlamebearerFormat(format || this.state.format);
  }

  fetchFlameBearerData(url) {
    if (this.currentJSONController) {
      this.currentJSONController.abort();
    }
    this.currentJSONController = new AbortController();

    fetch(`${url}&format=json`, { signal: this.currentJSONController.signal })
      .then((response) => response.json())
      .then((data) => {
        const { flamebearer } = data;
        deltaDiffWrapper(flamebearer.format, flamebearer.levels);

        this.setState({
          flamebearer,
        });
      })
      .finally();
  }

  render = () => {
    // This is necessary because the order switches depending on single vs comparison view
    const tablePane = (
      <div
        key="table-pane"
        className={clsx("pane", {
          hidden: this.state.view === "icicle",
          "vertical-orientation": this.props.viewType === "double",
        })}
      >
        <ProfilerTable
          flamebearer={this.state.flamebearer}
          sortByDirection={this.state.sortByDirection}
          sortBy={this.state.sortBy}
          updateSortBy={this.updateSortBy}
          view={this.state.view}
          viewDiff={this.state.viewDiff}
        />
      </div>
    );

    const flameGraphPane = this.state.flamebearer ? (
      <Graph flamebearer={this.state.flamebearer} />
    ) : null;

    const panes =
      this.props.viewType === "double"
        ? [flameGraphPane, tablePane]
        : [tablePane, flameGraphPane];

    // const flotData = this.props.timeline
    //   ? [this.props.timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
    //   : [];

    return (
      <div
        className={clsx("canvas-renderer", {
          double: this.props.viewType === "double",
        })}
      >
        <div className="canvas-container">
          <ProfilerHeader
            view={this.state.view}
            viewDiff={this.state.viewDiff}
            handleSearchChange={this.handleSearchChange}
            reset={this.reset}
            updateView={this.updateView}
            updateViewDiff={this.updateViewDiff}
            resetStyle={this.state.resetStyle}
          />
          {this.props.viewType === "double" ? (
            <>
              <InstructionText {...this.props} />
              <TimelineChartWrapper
                key={`timeline-chart-${this.props.viewSide}`}
                id={`timeline-chart-${this.props.viewSide}`}
                viewSide={this.props.viewSide}
              />
            </>
          ) : this.props.viewType === "diff" ? (
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
            className={clsx("flamegraph-container panes-wrapper", {
              "vertical-orientation": this.props.viewType === "double",
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
