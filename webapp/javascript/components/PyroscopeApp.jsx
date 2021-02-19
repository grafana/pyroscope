import React, { useState, useEffect, useRef } from "react";
import { connect } from "react-redux";
import "react-dom";

import { bindActionCreators } from "redux";
import FlameGraphRenderer from "./FlameGraphRenderer";
import TimelineChart from "./TimelineChart";
import Header from "./Header";
import Footer from "./Footer";
import { buildRenderURL } from "../util/updateRequests";
import { fetchNames, fetchTimeline } from "../redux/actions";

// See docs here: https://github.com/flot/flot/blob/master/API.md
const flotOptions = {
  margin: {
    top: 0,
    left: 0,
    bottom: 0,
    right: 0,
  },
  selection: {
    mode: "x",
  },
  crosshair: {
    mode: "x",
    color: "#C3170D",
    lineWidth: "1",
  },
  grid: {
    borderWidth: 1,
    margin: {
      left: 16,
      right: 16,
    },
  },
  yaxis: {
    show: false,
    min: 0,
  },
  points: {
    show: false,
    radius: 0.1,
  },
  lines: {
    show: false,
    steps: true,
    lineWidth: 1.0,
  },
  bars: {
    show: true,
    fill: true,
  },
  xaxis: {
    mode: "time",
    timezone: "browser",
    reserveSpace: false,
  },
};

function PyroscopeApp(props) {
  const { actions, renderURL, timeline } = props;
  const [state, setState] = useState(initialState);
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      actions.fetchTimeline(renderURL);
    }
  }, [renderURL]);

  const flotData = timeline
    ? [timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
    : [];

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChart
          id="timeline-chart"
          options={flotOptions}
          data={flotData}
          width="100%"
          height="100px"
        />
        <FlameGraphRenderer orientation="horizontal" />
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
  renderURL: buildRenderURL(state),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchTimeline,
      fetchNames,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(PyroscopeApp);
