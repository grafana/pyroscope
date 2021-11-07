import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import TimelineChartWrapper from './TimelineChartWrapper';
import Header from './Header';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import { fetchNames, fetchTimeline } from '../redux/actions';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function ComparisonApp(props) {
  const { actions, renderURL, leftRenderURL, rightRenderURL } = props;
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      // When ComparisonApp is loaded only renderURL is set so
      // by using viewType = 'comparison' and not giving a viewSide
      // populates both left and right side keys
      // as shown in redux/reducers/filters.js:RECEIVE_TIMELINE case.
      actions.fetchTimeline(renderURL, 'comparison');
    }

    if (prevPropsRef.leftRenderURL !== leftRenderURL) {
      actions.fetchTimeline(leftRenderURL, 'comparison', 'left');
    }

    if (prevPropsRef.rightRenderURL !== rightRenderURL) {
      actions.fetchTimeline(rightRenderURL, 'comparison', 'right');
    }

    return actions.abortTimelineRequest;
  }, [renderURL, leftRenderURL, rightRenderURL]);

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
            data-testid="flamegraph-renderer-left"
          />
          <FlameGraphRenderer
            viewType="double"
            viewSide="right"
            data-testid="flamegraph-renderer-right"
          />
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
