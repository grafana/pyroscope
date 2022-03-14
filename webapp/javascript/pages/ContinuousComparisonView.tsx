import React, { useEffect } from 'react';
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
import TagsBar from '../components/TagsBar';
import styles from './ContinuousComparison.module.css';

function ComparisonApp() {
  const dispatch = useAppDispatch();
  const {
    from,
    until,
    query,
    leftQuery,
    rightQuery,
    refreshToken,
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
  } = useAppSelector(selectContinuousState);

  const comparisonView = useAppSelector(selectComparisonState);

  useEffect(() => {
    dispatch(fetchInitialComparisonView(null));
  }, [query, refreshToken]);

  // When the page is first loaded, set a rightQuery if not existent
  useEffect(() => {
    if (!rightQuery && query) {
      dispatch(actions.setRightQuery(query));
    }
    if (!leftQuery && query) {
      dispatch(actions.setLeftQuery(query));
    }
  }, []);

  // timeline changes
  useEffect(() => {
    dispatch(fetchComparisonTimeline(null));
  }, [from, until]);

  // left side changes
  useEffect(() => {
    if (leftQuery) {
      dispatch(fetchComparisonSide({ side: 'left', query: leftQuery }));
    }
  }, [leftFrom, leftUntil, leftQuery, from, until]);

  // right side changes
  useEffect(() => {
    if (rightQuery) {
      dispatch(fetchComparisonSide({ side: 'right', query: rightQuery }));
    }
  }, [rightFrom, rightUntil, rightQuery, from, until]);

  //  const topTimeline = (() => {
  //    switch (comparisonView.timeline.type) {
  //      case 'loaded':
  //      case 'reloading': {
  //        return comparisonView.timeline.data;
  //      }
  //
  //      default:
  //        return undefined;
  //    }
  //  })();
  //
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

  const leftSide = getSide('left');
  const rightSide = getSide('right');

  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(
    leftSide.profile
  );
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(
    leftSide.profile
  );

  const leftTimeline = {
    color: 'rgba(200, 102, 204, 1)',
    data: leftSide.timeline,
  };

  const rightTimeline = {
    color: 'rgba(19, 152, 246, 1)',
    data: rightSide.timeline,
  };

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar hideTagsBar />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-double"
          viewSide="both"
          //          timeline={[topTimeline, leftSide.timeline, rightSide.timeline]}
          //          timeline={[topTimeline, leftSide.timeline, rightSide.timeline]}
          //          timeline={[leftSide.timeline, rightSide.timeline]}
          timeline={[leftTimeline, rightTimeline]}
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
            <TagsBar
              query={leftQuery}
              tags={[]}
              onSetQuery={(q) => {
                dispatch(actions.setLeftQuery(q));
              }}
            />
            <FlamegraphRenderer
              viewType="double"
              viewSide="left"
              profile={leftSide.profile}
              data-testid="flamegraph-renderer-left"
              display="both"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                leftSide.profile && (
                  <ExportData
                    flamebearer={leftSide.profile}
                    exportJSON
                    exportHTML
                    exportPprof
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComLeftFn}
                  />
                )
              }
            >
              <InstructionText viewType="double" viewSide="left" />
              <TimelineChartWrapper
                key="timeline-chart-left"
                id="timeline-chart-left"
                data-testid="timeline-left"
                viewSide="left"
                //                timeline={[
                //                  {
                //                    color: 'rgba(200, 102, 204, 1)',
                //                    data: leftSide.timeline,
                //                  },
                //                ]}
                timeline={[leftTimeline]}
                //                color="rgba(200, 102, 204, 1)"
                //                timeline={[leftSide.timeline, rightSide.timeline]}
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
            <TagsBar
              query={rightQuery}
              tags={[]}
              onSetQuery={(q) => {
                dispatch(actions.setRightQuery(q));
              }}
            />
            <FlamegraphRenderer
              viewType="double"
              viewSide="right"
              profile={rightSide.profile}
              data-testid="flamegraph-renderer-right"
              display="both"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                rightSide.profile && (
                  <ExportData
                    flamebearer={rightSide.profile}
                    exportJSON
                    exportHTML
                    exportPprof
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComRightFn}
                  />
                )
              }
            >
              <InstructionText viewType="double" viewSide="right" />
              <TimelineChartWrapper
                key="timeline-chart-right"
                id="timeline-chart-right"
                data-testid="timeline-right"
                viewSide="right"
                //                timeline={[leftSide.timeline, rightSide.timeline]}
                //                timeline={[
                //                  {
                //                    data: rightSide.timeline,
                //                    color: 'rgba(19, 152, 246, 0.35)',
                //                  },
                //                ]}
                timeline={[rightTimeline]}
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
