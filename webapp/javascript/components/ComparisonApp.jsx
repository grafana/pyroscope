import React, { useEffect, useRef } from "react";
import { connect } from "react-redux";
import "react-dom";

import { bindActionCreators } from "redux";
import FlameGraphRenderer from "./FlameGraphRenderer";
import TimelineChartWrapper from "./TimelineChartWrapper";
import Header from "./Header";
import Footer from "./Footer";
import { buildRenderURL } from "../util/updateRequests";
import { fetchNames, fetchTimeline } from "../redux/actions";

// See docs here: https://github.com/flot/flot/blob/master/API.md

function ComparisonApp(props) {
  const { actions, renderURL } = props;
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      actions.fetchTimeline(renderURL);
    }
  }, [renderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper id="timeline-chart-double" viewSide="both" />
        <div className="comparison-container">
          <FlameGraphRenderer viewType="double" viewSide="left" />
          <FlameGraphRenderer viewType="double" viewSide="right" />
        </div>
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

export default connect(mapStateToProps, mapDispatchToProps)(ComparisonApp);
