import React, { useEffect, useRef } from 'react';
import 'react-dom';

import Box from '@ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  selectContinuousState,
  actions,
  fetchInitialComparisonView,
  selectComparisonState,
  fetchComparisonSide,
  fetchComparisonTimeline,
} from '@pyroscope/redux/reducers/continuous';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import Toolbar from '../components/Toolbar';
import Footer from '../components/Footer';
import InstructionText from '../components/InstructionText';
import ExportData from '../components/ExportData';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import styles from './ContinuousComparison.module.css';

function ComparisonApp() {
  const dispatch = useAppDispatch();
  const {
    from,
    until,
    query,
    refreshToken,
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
  } = useAppSelector(selectContinuousState);

  const comparisonView = useAppSelector(selectComparisonState);

  useEffect(() => {
    dispatch(fetchInitialComparisonView());
  }, [query, refreshToken]);

  // timeline changes
  useEffect(() => {
    dispatch(fetchComparisonTimeline());
  }, [from, until]);

  // left side changes
  useEffect(() => {
    dispatch(fetchComparisonSide({ side: 'left' }));
  }, [leftFrom, leftUntil]);

  // right side changes
  useEffect(() => {
    dispatch(fetchComparisonSide({ side: 'right' }));
  }, [rightFrom, rightUntil]);

  const topTimeline = (() => {
    switch (comparisonView.timeline.type) {
      case 'loaded':
      case 'reloading': {
        return comparisonView.timeline.data;
      }

      default:
        return undefined;
    }
  })();

  const getSide = (side: 'left' | 'right') => {
    const s = comparisonView[side];

    switch (s.type) {
      case 'loaded':
      case 'reloading': {
        return s;
      }

      default:
        return { timeline: undefined, profile: undefined };
    }
  };

  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(
    getSide('left').profile
  );
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(
    getSide('right').profile
  );

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-double"
          viewSide="both"
          timeline={topTimeline}
          leftFrom={leftFrom}
          leftUntil={leftUntil}
          rightFrom={rightFrom}
          rightUntil={rightUntil}
          onSelect={(from, until) => {
            dispatch(actions.setFromAndUntil({ from, until }));
          }}
        />
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <FlamegraphRenderer
              viewType="double"
              viewSide="left"
              profile={getSide('left').profile}
              data-testid="flamegraph-renderer-left"
              display="both"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                <ExportData
                  flamebearer={getSide('left').profile}
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
                timeline={topTimeline}
                leftFrom={leftFrom}
                leftUntil={leftUntil}
                rightFrom={rightFrom}
                rightUntil={rightUntil}
                onSelect={(from, until) => {
                  dispatch(actions.setLeft({ from, until }));
                }}
              />
            </FlamegraphRenderer>
          </Box>

          <Box className={styles.comparisonPane}>
            <FlamegraphRenderer
              viewType="double"
              viewSide="right"
              profile={getSide('right').profile}
              data-testid="flamegraph-renderer-right"
              display="both"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                <ExportData
                  flamebearer={getSide('right').profile}
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
                timeline={topTimeline}
                leftFrom={leftFrom}
                leftUntil={leftUntil}
                rightFrom={rightFrom}
                rightUntil={rightUntil}
                onSelect={(from, until) => {
                  dispatch(actions.setRight({ from, until }));
                }}
              />
            </FlamegraphRenderer>
          </Box>
        </div>
      </div>
      <Footer />
    </div>
  );
}

export default ComparisonApp;
