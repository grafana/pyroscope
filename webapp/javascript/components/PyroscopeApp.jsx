import React from "react";
import { connect } from "react-redux";
import "react-dom";

import { bindActionCreators } from "redux";
import FlameGraphRenderer from "./FlameGraphRenderer";
import TimelineChartWrapper from "./TimelineChartWrapper";
import Header from "./Header";
import Footer from "./Footer";
import { buildRenderURL } from "../util/updateRequests";
import { fetchNames, fetchTimeline } from "../redux/actions";

function PyroscopeApp() {
  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper id="timeline-chart-single" viewSide="none" />
        <FlameGraphRenderer viewType="single" />
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
