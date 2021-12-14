import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import { setLeftFile, setRightFile } from '../redux/actions';
import styles from './ComparisonApp.module.css';

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
          <Box className={styles.comparisonPane}>
            <FlameGraphRenderer
              viewType="double"
              viewSide="left"
              flamebearer={leftFlamebearer}
              data-testid="flamegraph-renderer-left"
              uploader={{ file: leftFile, setFile: setLeftFile }}
            />
          </Box>
          <Box className={styles.comparisonPane}>
            <FlameGraphRenderer
              viewType="double"
              viewSide="right"
              flamebearer={rightFlamebearer}
              data-testid="flamegraph-renderer-right"
              uploader={{ file: rightFile, setFile: setRightFile }}
            />
          </Box>
        </div>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  leftFile: state.root.adhocComparison.left.file,
  leftFlamebearer: state.root.adhocComparison.left.flamebearer,
  rightFile: state.root.adhocComparison.right.file,
  rightFlamebearer: state.root.adhocComparison.right.flamebearer,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({ setLeftFile, setRightFile }, dispatch),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocComparison);
