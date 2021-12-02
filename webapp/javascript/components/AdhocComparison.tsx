import React, { useEffect, useState } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchComparisonAppData,
  fetchTimeline,
} from '../redux/actions';
import FileUploader from './FileUploader';
import onFileUpload from '../util/onFileUpload';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function AdhocComparison(props) {
  const { actions, renderURL, leftRenderURL, rightRenderURL, comparison } =
    props;

  const [leftFlamebearer, setLeftFlamebearer] = useState();
  const [rightFlamebearer, setRightFlamebearer] = useState();

  const onLeftUpload = (data) => onFileUpload(data, setLeftFlamebearer);
  const onRightUpload = (data) => onFileUpload(data, setRightFlamebearer);

  useEffect(() => {
    return actions.abortTimelineRequest;
  }, [leftRenderURL]);

  useEffect(() => {
    return actions.abortTimelineRequest;
  }, [rightRenderURL]);

  useEffect(() => {
    return actions.abortTimelineRequest;
  }, [renderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <FlameGraphRenderer
            viewType="double"
            viewSide="left"
            flamebearer={leftFlamebearer}
            data-testid="flamegraph-renderer-left"
            uploader={onLeftUpload}
          />
          <FlameGraphRenderer
            viewType="double"
            viewSide="right"
            flamebearer={rightFlamebearer}
            data-testid="flamegraph-renderer-right"
            uploader={onRightUpload}
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
      fetchTimeline,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocComparison);
