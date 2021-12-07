import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { setLeftFile, setRightFile } from '../redux/actions';

function AdhocComparison(props) {
  const { actions, leftFile, leftFlamebearer, rightFile, rightFlamebearer } =
    props;
  const { setLeftFile } = actions;
  const { setRightFile } = actions;

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
  actions: bindActionCreators({ setLeftFile, setRightFile }, dispatch),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocComparison);
