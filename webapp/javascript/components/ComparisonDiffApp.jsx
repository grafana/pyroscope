import React, { useEffect, useRef } from "react";
import { connect } from "react-redux";
import { bindActionCreators } from "redux";
import FlameGraphRenderer from "./FlameGraphRenderer";
import Header from "./Header";
import Footer from "./Footer";
import TimelineChartWrapper from "./TimelineChartWrapper";
import { buildDiffRenderURL } from "../util/updateRequests";
import { fetchNames, fetchTimeline } from "../redux/actions";

function ComparisonDiffApp(props) {
  const { actions, diffRenderURL } = props;
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.diffRenderURL !== diffRenderURL) {
      actions.fetchTimeline(diffRenderURL);
    }
  }, [diffRenderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper id="timeline-chart-diff" viewSide="both" />
        <FlameGraphRenderer viewType="diff" />
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
  diffRenderURL: buildDiffRenderURL(state),
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

export default connect(mapStateToProps, mapDispatchToProps)(ComparisonDiffApp);
