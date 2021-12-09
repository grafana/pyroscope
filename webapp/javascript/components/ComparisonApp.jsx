import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import TimelineChartWrapper from './TimelineChartWrapper';
import Header from './Header';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchComparisonAppData,
  fetchTimeline,
} from '../redux/actions';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function ComparisonApp(props) {
  const { actions, renderURL, leftRenderURL, rightRenderURL, comparison } =
    props;

  useEffect(() => {
    actions.fetchComparisonAppData(leftRenderURL, 'left');
    return actions.abortTimelineRequest;
  }, [leftRenderURL]);

  useEffect(() => {
    actions.fetchComparisonAppData(rightRenderURL, 'right');
    return actions.abortTimelineRequest;
  }, [rightRenderURL]);

  useEffect(() => {
    actions.fetchTimeline(renderURL);

    return actions.abortTimelineRequest;
  }, [renderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper id="timeline-chart-double" viewSide="both" />
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <FlameGraphRenderer
            viewType="double"
            viewSide="left"
            flamebearer={comparison.left.flamebearer}
            data-testid="flamegraph-renderer-left"
          />
          <FlameGraphRenderer
            viewType="double"
            viewSide="right"
            flamebearer={comparison.right.flamebearer}
            data-testid="flamegraph-renderer-right"
          />
        </div>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  renderURL: buildRenderURL(state.root),
  leftRenderURL: buildRenderURL(
    state.root,
    state.root.leftFrom,
    state.root.leftUntil
  ),
  rightRenderURL: buildRenderURL(
    state.root,
    state.root.rightFrom,
    state.root.rightUntil
  ),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchComparisonAppData,
      fetchNames,
      fetchTimeline,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(ComparisonApp);
