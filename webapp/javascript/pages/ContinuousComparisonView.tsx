import React, { useEffect } from 'react';
import 'react-dom';

import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  selectContinuousState,
  actions,
  selectComparisonState,
  fetchComparisonSide,
  fetchTagValues,
} from '@webapp/redux/reducers/continuous';
import TimelineChartWrapper from '@webapp/components/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import Footer from '@webapp/components/Footer';
import InstructionText from '@webapp/components/InstructionText';
import ExportData from '@webapp/components/ExportData';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import TagsBar from '@webapp/components/TagsBar';
import styles from './ContinuousComparison.module.css';
import useTags from '../hooks/tags.hook';
import useTimelines, { leftColor, rightColor } from '../hooks/timeline.hook';
import usePopulateLeftRightQuery from '../hooks/populateLeftRightQuery.hook';

function ComparisonApp() {
  const dispatch = useAppDispatch();
  const { leftQuery, rightQuery, leftFrom, rightFrom, leftUntil, rightUntil } =
    useAppSelector(selectContinuousState);

  usePopulateLeftRightQuery();
  const comparisonView = useAppSelector(selectComparisonState);
  const { leftTags, rightTags } = useTags({ leftQuery, rightQuery });
  const { leftTimeline, rightTimeline } = useTimelines();

  useEffect(() => {
    if (leftQuery) {
      dispatch(fetchComparisonSide({ side: 'left', query: leftQuery }));
    }
  }, [leftFrom, leftUntil, leftQuery]);

  useEffect(() => {
    if (rightQuery) {
      dispatch(fetchComparisonSide({ side: 'right', query: rightQuery }));
    }
  }, [rightFrom, rightUntil, rightQuery]);

  const leftSide = comparisonView.left.profile;
  const rightSide = comparisonView.right.profile;
  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(leftSide);
  const exportToFlamegraphDotComRightFn =
    useExportToFlamegraphDotCom(rightSide);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar
          hideTagsBar
          onSelectedName={(query) => {
            dispatch(actions.setRightQuery(query));
            dispatch(actions.setLeftQuery(query));
            dispatch(actions.setQuery(query));
          }}
        />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-double"
          format="lines"
          timelineA={leftTimeline}
          timelineB={rightTimeline}
          onSelect={(from, until) => {
            dispatch(actions.setFromAndUntil({ from, until }));
          }}
          markings={{
            left: { from: leftFrom, to: leftUntil, color: leftColor },
            right: { from: rightFrom, to: rightUntil, color: rightColor },
          }}
        />
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <TagsBar
              query={leftQuery || ''}
              tags={leftTags}
              onSetQuery={(q) => {
                dispatch(actions.setLeftQuery(q));
              }}
              onSelectedLabel={(label, query) => {
                dispatch(fetchTagValues({ query, label }));
              }}
            />
            <FlamegraphRenderer
              panesOrientation="vertical"
              profile={leftSide}
              data-testid="flamegraph-renderer-left"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                leftSide && (
                  <ExportData
                    flamebearer={leftSide}
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
                timelineA={leftTimeline}
                markings={{
                  left: { from: leftFrom, to: leftUntil, color: leftColor },
                }}
                onSelect={(from, until) => {
                  dispatch(actions.setLeft({ from, until }));
                }}
              />
            </FlamegraphRenderer>
          </Box>

          <Box className={styles.comparisonPane}>
            <TagsBar
              query={rightQuery || ''}
              tags={rightTags}
              onSetQuery={(q) => {
                dispatch(actions.setRightQuery(q));
              }}
              onSelectedLabel={(label, query) => {
                dispatch(fetchTagValues({ query, label }));
              }}
            />
            <FlamegraphRenderer
              profile={rightSide}
              data-testid="flamegraph-renderer-right"
              panesOrientation="vertical"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                rightSide && (
                  <ExportData
                    flamebearer={rightSide}
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
                timelineA={rightTimeline}
                markings={{
                  right: { from: rightFrom, to: rightUntil, color: rightColor },
                }}
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
