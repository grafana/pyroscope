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
  fetchTimeline,
  abortTimelineRequest,
} from '../redux/actions';

function PyroscopeApp(props) {
  const { actions, renderURL } = props;
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      actions.fetchTimeline(renderURL);
    }

    return actions.abortTimelineRequest;
  }, [renderURL]);

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
      abortTimelineRequest,
    },
    dispatch,
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(PyroscopeApp);
