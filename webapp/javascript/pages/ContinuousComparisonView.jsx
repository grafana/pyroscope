import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import Header from '../components/Header';
import Footer from '../components/Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchComparisonAppData,
  fetchTimeline,
} from '../redux/actions';
import InstructionText from '../components/InstructionText';
import ExportData from '../components/ExportData';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import styles from './ContinuousComparison.module.css';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function ComparisonApp(props) {
  const { actions, renderURL, leftRenderURL, rightRenderURL, comparison } =
    props;
  const { rawLeft, rawRight } = comparison;
  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(rawLeft);
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(rawRight);

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
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-double"
          viewSide="both"
        />
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <FlamegraphRenderer
              viewType="double"
              viewSide="left"
              flamebearer={comparison.left.flamebearer}
              data-testid="flamegraph-renderer-left"
              display="both"
              rawFlamegraph={rawLeft}
              ExportData={
                // Don't export PNG since the exportPng code is broken
                <ExportData
                  flamebearer={rawLeft}
                  exportJSON
                  exportHTML
                  exportPprof
                  exportFlamegraphDotCom
                  exportFlamegraphDotComFn={exportToFlamegraphDotComLeftFn}
                />
              }
            >
              <InstructionText viewType="double" viewSide="left" />
              <TimelineChartWrapper
                key="timeline-chart-left"
                id="timeline-chart-left"
                data-testid="timeline-left"
                viewSide="left"
              />
            </FlamegraphRenderer>
          </Box>

          <Box className={styles.comparisonPane}>
            <FlamegraphRenderer
              viewType="double"
              viewSide="right"
              flamebearer={comparison.right.flamebearer}
              data-testid="flamegraph-renderer-right"
              display="both"
              rawFlamegraph={rawRight}
              ExportData={
                // Don't export PNG since the exportPng code is broken
                <ExportData
                  flamebearer={rawRight}
                  exportJSON
                  exportHTML
                  exportPprof
                  exportFlamegraphDotCom
                  exportFlamegraphDotComFn={exportToFlamegraphDotComRightFn}
                />
              }
            >
              <InstructionText viewType="double" viewSide="right" />
              <TimelineChartWrapper
                key="timeline-chart-right"
                id="timeline-chart-right"
                data-testid="timeline-right"
                viewSide="right"
              />
            </FlamegraphRenderer>
          </Box>
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
