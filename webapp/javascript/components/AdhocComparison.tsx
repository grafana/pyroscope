import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { abortTimelineRequest, setLeftFile, setRightFile } from '../redux/actions';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function AdhocComparison(props) {
  const { actions, leftFile, leftFlamebearer, rightFile, rightFlamebearer } = props;
  const setLeftFile = actions.setLeftFile;
  const setRightFile = actions.setRightFile;

  useEffect(() => {
    return actions.abortTimelineRequest;
  });

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
            uploader={{ file: leftFile, setFile: setLeftFile }}
          />
          <FlameGraphRenderer
            viewType="double"
            viewSide="right"
            flamebearer={rightFlamebearer}
            data-testid="flamegraph-renderer-right"
            uploader={{ file: rightFile, setFile: setRightFile }}
          />
        </div>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
  leftFile: state.adhocComparison.left.file,
  leftFlamebearer: state.adhocComparison.left.flamebearer,
  rightFile: state.adhocComparison.right.file,
  rightFlamebearer: state.adhocComparison.right.flamebearer,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({ abortTimelineRequest, setLeftFile, setRightFile }, dispatch)
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocComparison);
