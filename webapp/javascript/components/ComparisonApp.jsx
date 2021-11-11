import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import TimelineChartWrapper from './TimelineChartWrapper';
import Header from './Header';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import { fetchNames, fetchComparisonAppData } from '../redux/actions';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function ComparisonApp(props) {
  const { actions, renderURL, leftRenderURL, rightRenderURL, comparison } =
    props;
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      actions.fetchComparisonAppData(renderURL, 'both');
    }

    if (prevPropsRef.leftRenderURL !== leftRenderURL) {
      actions.fetchComparisonAppData(leftRenderURL, 'left');
    }

    if (prevPropsRef.rightRenderURL !== rightRenderURL) {
      actions.fetchComparisonAppData(rightRenderURL, 'right');
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
  ...state,
  renderURL: buildRenderURL(state),
  leftRenderURL: buildRenderURL(state, state.leftFrom, state.leftUntil),
  rightRenderURL: buildRenderURL(state, state.rightFrom, state.rightUntil),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchComparisonAppData,
      fetchNames,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(ComparisonApp);
